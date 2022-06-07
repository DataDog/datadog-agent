#!/bin/bash

function cleanup {
    rm -rf "$TDIR"
}

set -e

TDIR=$(mktemp -d)
trap cleanup EXIT
cd $TDIR

wget https://curl.se/download/curl-7.83.1.tar.gz
tar xzf curl-7.83.1.tar.gz
cd curl-7.83.1

#  --with-gssapi \
#  --with-libssh2 \

./configure \
  --disable-ldap \
  --disable-ldaps \
  --disable-manual \
  --enable-ipv6 \
  --enable-threaded-resolver \
  --with-openssl \
  --with-gnutls \
  --with-random='/dev/urandom'
#\
#  --with-ca-bundle='/etc/ssl/certs/ca-certificates.crt'

make -j8

./src/curl --version
