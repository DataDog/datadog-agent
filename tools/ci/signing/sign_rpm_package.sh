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

macros_path="/root/.rpmmacros"
cat <<MACRO | tee "${macros_path}"
%_signature gpg
%_gpg_name ${keyid}
%__gpg /usr/bin/gpg

%__gpg_sign_cmd %{__gpg} \
  --no-armor --digest-algo sha256 -u "%{_gpg_name}" -b -o %{__signature_filename} \
  %{__plaintext_filename}

# These are SHA256 - we use them to build packages installable in FIPS mode
%_source_filedigest_algorithm 8
%_binary_filedigest_algorithm 8
MACRO

if ! rpm --addsign "${tmpfile}"; then
  log "Failed to sign RPM file ${tmpfile}" ERROR
  exit 1
else
  log "Successfully signed RPM file ${tmpfile}" INFO
fi

mkdir -p "${outdir}"

cp "${tmpfile}" "${outdir}/"

rm -rf "${tmpdir}"
