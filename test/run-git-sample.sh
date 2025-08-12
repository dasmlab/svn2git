#!/usr/bin/env bash
set -euo pipefail

# Starts a minimal Git HTTP server using Gitea in Docker on a shared network.

NET_NAME="scm-playground"
GIT_CONT="git-sample"
GIT_PORT_HTTP=3000
GIT_PORT_SSH=2222

docker network inspect "$NET_NAME" >/dev/null 2>&1 || docker network create "$NET_NAME"

if docker ps -a --format '{{.Names}}' | grep -q "^${GIT_CONT}$"; then
  docker rm -f "$GIT_CONT" >/dev/null
fi

IMAGE="gitea/gitea:1.21"
docker pull "$IMAGE" >/dev/null

GIT_DATA=$(mktemp -d)
trap 'rm -rf "$GIT_DATA"' EXIT

docker run -d --name "$GIT_CONT" --network "$NET_NAME" \
  -p ${GIT_PORT_HTTP}:3000 -p ${GIT_PORT_SSH}:22 \
  -e USER_UID=1000 -e USER_GID=1000 \
  -e GITEA__server__DOMAIN=localhost \
  -e GITEA__server__ROOT_URL=http://localhost:${GIT_PORT_HTTP}/ \
  -e GITEA__security__INSTALL_LOCK=true \
  -v "$GIT_DATA":/data "$IMAGE" >/dev/null

echo "Waiting for Gitea to be ready..."
until curl -fsS http://localhost:${GIT_PORT_HTTP}/api/health >/dev/null 2>&1; do sleep 1; done

echo "Gitea running: http://localhost:${GIT_PORT_HTTP} (network: ${NET_NAME}, container: ${GIT_CONT})"
echo "An admin user and repo can be created via: docker exec ${GIT_CONT} gitea admin user create ... and gitea admin repo create ..."

