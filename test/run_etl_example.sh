#!/usr/bin/env bash
set -euo pipefail

# Orchestrates SVN -> svn2git -> Git example end-to-end.
# - Starts SVN and populates sample repo
# - Starts Git server (Gitea)
# - Exports SVN working copy to host
# - Builds and runs svn2git container to push snapshot

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
NET_NAME="scm-playground"
SVN_CONT="svn-sample"
GIT_CONT="git-sample"
WC_HOST_DIR=/tmp/svn-wc

echo "[1/5] Starting SVN sample..."
"$ROOT_DIR"/test/run-svn-sample.sh

echo "[2/5] Starting Git sample..."
"$ROOT_DIR"/test/run-git-sample.sh

echo "[3/5] Exporting SVN working copy to host at ${WC_HOST_DIR}..."
mkdir -p "$WC_HOST_DIR"
docker exec "$SVN_CONT" bash -lc 'tar -C /tmp/wc -cf - .' | tar -C "$WC_HOST_DIR" -xf -

echo "[4/5] Building svn2git container..."
docker build -t local/svn2git:dev "$ROOT_DIR"

GIT_REMOTE=${GIT_REMOTE:-http://git-sample:3000/<user>/<repo>.git}
GIT_USER=${GIT_USER:-user}
GIT_PASSWORD=${GIT_PASSWORD:-pass}

echo "[5/5] Running svn2git ETL..."
docker run --rm --network "$NET_NAME" -v "$WC_HOST_DIR":/src:ro local/svn2git:dev \
  --source /src --target "$GIT_REMOTE" --user "$GIT_USER" --password "$GIT_PASSWORD" --debug

echo "Done. Verify repository at $GIT_REMOTE."

