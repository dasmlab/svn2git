#!/usr/bin/env bash
set -euo pipefail

# Starts a throwaway SVN server in Docker, initializes a repo with sample data,
# and exposes it on a shared docker network for integration testing.

NET_NAME="scm-playground"
SVN_CONT="svn-sample"
SVN_PORT=3690

docker network inspect "$NET_NAME" >/dev/null 2>&1 || docker network create "$NET_NAME"

# Clean up any previous container
if docker ps -a --format '{{.Names}}' | grep -q "^${SVN_CONT}$"; then
  docker rm -f "$SVN_CONT" >/dev/null
fi

# Use a lightweight svn server image
IMAGE="elleflorio/svn-server:latest"
docker pull "$IMAGE" >/dev/null

DATA_DIR=$(mktemp -d)
trap 'rm -rf "$DATA_DIR"' EXIT

# Prepare sample repo structure (tree depth, random and real files)
mkdir -p "$DATA_DIR/repo"
cat >"$DATA_DIR/create.sh" <<'EOS'
#!/usr/bin/env bash
set -euo pipefail
repo=/var/opt/svn/repo
svnadmin create "$repo"
svnserve -d -r /var/opt/svn
mkdir -p /tmp/wc
svn co svn://localhost/repo /tmp/wc
pushd /tmp/wc >/dev/null
  mkdir -p src/lib/utils docs/assets/images
  echo "hello world" > README.md
  head -c 128 </dev/urandom > docs/assets/random.bin
  for i in $(seq 1 5); do echo "file $i" > src/file$i.txt; done
  echo '{"name":"example","version":"1.0.0"}' > src/lib/utils/data.json
  dd if=/dev/urandom of=src/lib/big.dat bs=1K count=64 2>/dev/null
  svn add .
  svn ci -m "Initial import with structured depth and random data"
  mkdir -p deep/nested/path/levels
  echo "deep" > deep/nested/path/levels/leaf.txt
  svn add deep
  svn ci -m "Add deep nested structure"
popd >/dev/null
EOS
chmod +x "$DATA_DIR/create.sh"

docker run -d --name "$SVN_CONT" --network "$NET_NAME" -p ${SVN_PORT}:3690 \
  -v "$DATA_DIR":/init elleflorio/svn-server:latest tail -f /dev/null >/dev/null

# Initialize repo content inside the container
docker exec -u root "$SVN_CONT" bash -lc 'apt-get update >/dev/null && apt-get install -y subversion >/dev/null'
docker exec "$SVN_CONT" bash /init/create.sh

echo "SVN server running: svn://localhost:${SVN_PORT}/repo (network: ${NET_NAME}, container: ${SVN_CONT})"

