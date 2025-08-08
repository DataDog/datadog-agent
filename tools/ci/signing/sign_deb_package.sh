#!/usr/bin/env bash

set -euo pipefail

package=$1
keyid=$2
outdir=$3

log() {
  local message=$1     # what to log, as a single string
  local error_level=$2 # the log level that will prefix the message

  echo "${error_level} [$(date "+%Y.%m.%d-%H:%M:%S %Z")] ${message}" 1>&2
}

tmpdir=$(mktemp -d)
tmpfile="${tmpdir}"/$(basename "${package}")

cp "${package}" "${tmpfile}"

# The image we run in has ar 2.30, which always extracts to the current directory.
pushd "${tmpdir}" && ar x "${tmpfile}" && popd

cat "${tmpdir}"/debian-binary "${tmpdir}"/control.tar.* "${tmpdir}"/data.tar.* > "${tmpdir}"/complete

if ! (fakeroot gpg --armor --detach-sign --local-user "${keyid}" \
    -o "${tmpdir}"/_gpgorigin "${tmpdir}"/complete); then
    log "Failed to sign DEB file ${tmpfile}" ERROR
    exit 1
else
    log "Successfully signed DEB file ${tmpfile}" INFO
fi

if ! fakeroot ar rc "${tmpfile}" "${tmpdir}/_gpgorigin"; then
    log "Failed to add signature to DEB file ${tmpfile}" ERROR
    exit 1
fi

mkdir -p "${outdir}"

cp "${tmpfile}" "${outdir}/"

rm -rf "${tmpdir}"