package etl

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	httpgit "github.com/go-git/go-git/v5/plumbing/transport/http"
	log "github.com/sirupsen/logrus"
)

// Snapshot represents a simple file tree export without SVN metadata.
type Snapshot struct {
	RootPath string
	Files    []SnapshotFile
}

// SnapshotFile contains relative path and a src absolute path to copy bytes from.
type SnapshotFile struct {
	RelPath string
	SrcPath string
	Size    int64
}

// Manifest captures derived information useful for auditing the ETL run.
type Manifest struct {
	CreatedAt  time.Time
	SourceRoot string
	FileCount  int
	TotalBytes int64
}

// ExtractSnapshotFromWorkingCopy walks the source directory and builds a Snapshot.
// It skips the .svn admin directories to avoid copying metadata.
func ExtractSnapshotFromWorkingCopy(sourceRoot string) (*Snapshot, *Manifest, error) {
	info, err := os.Stat(sourceRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("stat source: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("source must be a directory: %s", sourceRoot)
	}

	var files []SnapshotFile
	var total int64
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), ".svn") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, SnapshotFile{RelPath: filepath.ToSlash(rel), SrcPath: path, Size: st.Size()})
		total += st.Size()
		return nil
	}
	if err := filepath.WalkDir(sourceRoot, walkFn); err != nil {
		return nil, nil, fmt.Errorf("walk source: %w", err)
	}
	manifest := &Manifest{
		CreatedAt:  time.Now(),
		SourceRoot: sourceRoot,
		FileCount:  len(files),
		TotalBytes: total,
	}
	snapshot := &Snapshot{RootPath: sourceRoot, Files: files}
	return snapshot, manifest, nil
}

// TransformOptions control how we stage the Git repo.
type TransformOptions struct {
	WorkDir  string
	Author   string
	Email    string
	Message  string
	Manifest *Manifest
}

// TransformToGitRepo creates a new ephemeral git repository and commits the snapshot.
func TransformToGitRepo(snapshot *Snapshot, opts TransformOptions) (string, *git.Repository, error) {
	workdir := opts.WorkDir
	if workdir == "" {
		dir, err := os.MkdirTemp("", "svn2git-*")
		if err != nil {
			return "", nil, fmt.Errorf("create temp dir: %w", err)
		}
		workdir = dir
	} else {
		if err := os.MkdirAll(workdir, 0o755); err != nil {
			return "", nil, fmt.Errorf("ensure workdir: %w", err)
		}
	}

	repoPath := filepath.Join(workdir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return "", nil, fmt.Errorf("create repo dir: %w", err)
	}

	// Initialize repository
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		return "", nil, fmt.Errorf("init repo: %w", err)
	}

	// Copy files into repo working tree
	wt, err := repo.Worktree()
	if err != nil {
		return "", nil, fmt.Errorf("get worktree: %w", err)
	}

	for _, f := range snapshot.Files {
		destPath := filepath.Join(repoPath, filepath.FromSlash(f.RelPath))
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(destPath), err)
		}
		if err := copyFileContents(f.SrcPath, destPath); err != nil {
			return "", nil, fmt.Errorf("copy %s: %w", f.RelPath, err)
		}
		if _, err := wt.Add(f.RelPath); err != nil {
			return "", nil, fmt.Errorf("git add %s: %w", f.RelPath, err)
		}
	}

	// Add manifest file for traceability
	if opts.Manifest != nil {
		manifestPath := filepath.Join(repoPath, "SVN_IMPORT_MANIFEST.txt")
		content := buildManifestText(opts.Manifest)
		if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
			return "", nil, fmt.Errorf("write manifest: %w", err)
		}
		if _, err := wt.Add("SVN_IMPORT_MANIFEST.txt"); err != nil {
			return "", nil, fmt.Errorf("git add manifest: %w", err)
		}
	}

	// Commit
	commitMsg := opts.Message
	if commitMsg == "" {
		commitMsg = "SVN import"
	}
	sig := &object.Signature{Name: opts.Author, Email: opts.Email, When: time.Now()}
	if _, err := wt.Commit(commitMsg, &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		return "", nil, fmt.Errorf("commit: %w", err)
	}

	return repoPath, repo, nil
}

// LoadPushToRemote adds a remote and pushes to it over HTTPS.
func LoadPushToRemote(repo *git.Repository, remoteURL, username, password string, tlsConfig *tls.Config) error {
	if repo == nil {
		return errors.New("repo is nil")
	}
	if remoteURL == "" {
		return errors.New("remoteURL is empty")
	}

	// Create or update origin remote
	_, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteURL}})
	if err != nil && !errors.Is(err, git.ErrRemoteExists) {
		return fmt.Errorf("create remote: %w", err)
	}

	// Build auth
	var auth transport.AuthMethod
	if username != "" || password != "" {
		auth = &httpgit.BasicAuth{Username: username, Password: password}
	}
	// Optionally tweak TLS via a custom net/http client used by go-git
	if tlsConfig != nil {
		netClient := &nethttp.Client{Transport: &nethttp.Transport{TLSClientConfig: tlsConfig}}
		httpgit.DefaultClient = httpgit.NewClient(netClient)
	}

	// Push
	pushErr := repo.Push(&git.PushOptions{RemoteName: "origin", Auth: auth})
	if pushErr == git.NoErrAlreadyUpToDate {
		log.Info("remote already up-to-date")
		return nil
	}
	if pushErr != nil {
		return fmt.Errorf("push: %w", pushErr)
	}
	return nil
}

func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func buildManifestText(m *Manifest) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "SVN -> GIT IMPORT MANIFEST\n")
	fmt.Fprintf(b, "Created: %s\n", m.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(b, "SourceRoot: %s\n", m.SourceRoot)
	fmt.Fprintf(b, "FileCount: %d\n", m.FileCount)
	fmt.Fprintf(b, "TotalBytes: %d\n", m.TotalBytes)
	return b.String()
}
