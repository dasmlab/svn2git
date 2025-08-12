#!/usr/bin/env bash
set -euo pipefail

# Starts a minimal Git HTTP server using Gogs in Docker on a shared network.

NET_NAME="scm-playground"
GIT_CONT="git-sample"
GIT_PORT_HTTP=3000
GIT_PORT_SSH=2222

docker network inspect "$NET_NAME" >/dev/null 2>&1 || docker network create "$NET_NAME"

if docker ps -a --format '{{.Names}}' | grep -q "^${GIT_CONT}$"; then
  docker rm -f "$GIT_CONT" >/dev/null
fi

IMAGE="gogs/gogs:latest"
docker pull "$IMAGE" >/dev/null

GIT_DATA=$(mktemp -d)
trap 'rm -rf "$GIT_DATA"' EXIT

docker run -d --name "$GIT_CONT" --network "$NET_NAME" \
  -p ${GIT_PORT_HTTP}:3000 -p ${GIT_PORT_SSH}:22 \
  -v "$GIT_DATA":/data "$IMAGE" >/dev/null

echo "Waiting for Gogs to be ready..."
until curl -fsS http://localhost:${GIT_PORT_HTTP}/ >/dev/null; do sleep 1; done

echo "Gogs running: http://localhost:${GIT_PORT_HTTP} (network: ${NET_NAME}, container: ${GIT_CONT})"
echo "Create a repo via UI named 'repo' and use: http://localhost:${GIT_PORT_HTTP}/<user>/repo.git"

