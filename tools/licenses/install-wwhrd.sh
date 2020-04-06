#!/usr/bin/env bash
set -e

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(git rev-parse --show-toplevel)
WORK_DIR=`mktemp -d`
cleanup() {
  rm -rf "$WORK_DIR"
}
trap "cleanup" EXIT SIGINT

VERSION=${1:-0.2.4}
TARBALL="wwhrd_${VERSION}_$(uname)_amd64.tar.gz"

cd $WORK_DIR
curl -Lo ${TARBALL} https://github.com/frapposelli/wwhrd/releases/download/v${VERSION}/${TARBALL} && tar -C . -xzf $TARBALL

chmod +x wwhrd
mkdir -p $ROOT/bin
mv wwhrd $ROOT/bin/wwhrd
