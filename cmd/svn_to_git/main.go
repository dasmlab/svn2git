package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/dasmlab/svn-2-git/internal/etl"
)

type cliArgs struct {
	sourcePath    string
	targetURL     string
	username      string
	password      string
	authorName    string
	authorEmail   string
	commitMessage string
	workdir       string
	debug         bool
	dryRun        bool
	insecureTLS   bool
}

func parseArgs() *cliArgs {
	args := &cliArgs{}
	flag.StringVar(&args.sourcePath, "source", "", "Path to SVN working copy to export (snapshot). The directory is walked and .svn is ignored.")
	flag.StringVar(&args.targetURL, "target", "", "Target Git remote URL (https://...).")
	flag.StringVar(&args.username, "user", os.Getenv("GIT_USER"), "Git username (can also use env GIT_USER).")
	flag.StringVar(&args.password, "password", os.Getenv("GIT_PASSWORD"), "Git password or token (can also use env GIT_PASSWORD).")
	flag.StringVar(&args.authorName, "author-name", os.Getenv("GIT_AUTHOR_NAME"), "Commit author name (default: same as user).")
	flag.StringVar(&args.authorEmail, "author-email", os.Getenv("GIT_AUTHOR_EMAIL"), "Commit author email (optional).")
	flag.StringVar(&args.commitMessage, "message", "Import from SVN snapshot", "Commit message to use for the import.")
	flag.StringVar(&args.workdir, "workdir", "", "Optional working directory for building the temporary git repo. Defaults to a temp dir.")
	flag.BoolVar(&args.debug, "debug", false, "Enable debug logging.")
	flag.BoolVar(&args.dryRun, "dry-run", false, "Perform extract and transform only; do not push to remote.")
	flag.BoolVar(&args.insecureTLS, "insecure", false, "Do not verify TLS certificates when pushing over HTTPS (use with caution).")
	flag.Parse()
	return args
}

func main() {
	args := parseArgs()

	// Configure logging
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	if args.debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if args.sourcePath == "" || args.targetURL == "" {
		fmt.Println("usage: ./svn_to_git --source <svn working copy path> --target <git remote url> --user <username> --password <password>")
		flag.PrintDefaults()
		os.Exit(2)
	}

	if args.authorName == "" {
		// use username as author name by default
		args.authorName = args.username
	}

	start := time.Now()
	log.WithFields(log.Fields{
		"source": args.sourcePath,
		"target": args.targetURL,
		"user":   args.username,
		"dryRun": args.dryRun,
	}).Info("starting SVN -> Git snapshot import")

	// Extract
	snapshot, manifest, err := etl.ExtractSnapshotFromWorkingCopy(args.sourcePath)
	if err != nil {
		log.WithError(err).Fatal("extract step failed")
	}
	log.WithFields(log.Fields{"files": len(snapshot.Files), "bytes": manifest.TotalBytes}).Info("extracted snapshot")

	// Transform
	repoPath, repo, err := etl.TransformToGitRepo(snapshot, etl.TransformOptions{
		WorkDir:  args.workdir,
		Author:   args.authorName,
		Email:    args.authorEmail,
		Message:  args.commitMessage,
		Manifest: manifest,
	})
	if err != nil {
		log.WithError(err).Fatal("transform step failed")
	}
	log.WithFields(log.Fields{"repo": repoPath}).Info("prepared git repository")

	if args.dryRun {
		log.Warn("dry-run enabled; skipping push to remote")
		log.WithField("elapsed", time.Since(start).String()).Info("completed (dry-run)")
		return
	}

	// Load
	var tlsConfig *tls.Config
	if args.insecureTLS {
		tlsConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user requested explicitly
	}
	if err := etl.LoadPushToRemote(repo, args.targetURL, args.username, args.password, tlsConfig); err != nil {
		log.WithError(err).Fatal("load step failed (push)")
	}

	log.WithField("elapsed", time.Since(start).String()).Info("completed SVN -> Git import")
}
