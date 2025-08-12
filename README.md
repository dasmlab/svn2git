# svn-2-git (snapshot importer)

A portable Go program that snapshots a local SVN working copy and pushes it to a Git remote over HTTPS. No external binaries required.

## Build

```bash
make tidy
make build
```

## Usage

```bash
./bin/svn_to_git --source /path/to/svn/working-copy \
  --target https://git.example.com/owner/repo.git \
  --user myuser --password mytoken
```

Optional flags:
- `--author-name`, `--author-email`
- `--message`
- `--workdir` (defaults to temp)
- `--dry-run`
- `--debug`
- `--insecure` (skip TLS verify)

The tool performs an ETL pipeline:
- Extract: walk the working copy, ignoring `.svn`.
- Transform: initialize a Git repo, copy files, create a single commit, add an `SVN_IMPORT_MANIFEST.txt` for traceability.
- Load: push to the remote via HTTPS using basic auth.

Environment variables: `GIT_USER`, `GIT_PASSWORD`, `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`.

Limitations: this performs a one-shot snapshot import; it does not attempt to preserve SVN history or annotations.

## Docker

Build container image:
```bash
docker build -t local/svn2git:dev .
```

Run in container:
```bash
docker run --rm -it \
  -v /path/to/working-copy:/src:ro \
  local/svn2git:dev \
  --source /src --target https://git.example.com/owner/repo.git \
  --user "$GIT_USER" --password "$GIT_PASSWORD"
```

## Test infrastructure

Spin up sample servers:
```bash
./test/run-svn-sample.sh
./test/run-git-sample.sh
```

Then run the import against the sample Git server URL created in the UI.

```mermaid
flowchart LR
  A[SVN Working Copy] -->|Extract| B[Snapshot]
  B -->|Transform| C[Ephemeral Git Repo]
  C -->|Load (HTTPS Push)| D[(Git Server)]
  C --> M[Manifest\nSVN_IMPORT_MANIFEST.txt]
```


