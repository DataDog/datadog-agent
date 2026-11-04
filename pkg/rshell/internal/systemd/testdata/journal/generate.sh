#!/bin/sh

set -eu

fixture_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
remote=${SYSTEMD_JOURNAL_REMOTE:-/usr/lib/systemd/systemd-journal-remote}

if [ "$#" -ne 1 ]; then
	echo "usage: $0 OUTPUT_DIR" >&2
	exit 2
fi

if [ ! -x "$remote" ]; then
	echo "systemd-journal-remote not found at $remote" >&2
	exit 1
fi

output_dir=$1
mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM
export_file="$tmpdir/fixture.export"
cp "$fixture_dir/fixture.export" "$export_file"
# Journal Export Format terminates every record, including the last, with an
# empty line. Keep that delimiter out of the tracked source's trailing space.
printf '\n' >>"$export_file"

normalize_machine_id() {
	# journal-remote uses the generator host's ID in the file header. Keep the
	# fixture synthetic and consistent with fixture.export instead.
	printf '\252\252\252\252\252\252\252\252\252\252\252\252\252\252\252\252' |
		dd of="$1" bs=1 seek=40 conv=notrunc status=none
}

SYSTEMD_JOURNAL_COMPACT=0 \
	SYSTEMD_JOURNAL_KEYED_HASH=0 \
	SYSTEMD_JOURNAL_COMPRESS=none \
	"$remote" \
	--split-mode=none \
	--compress=no \
	--output="$tmpdir/regular-uncompressed.journal" \
	"$export_file"

SYSTEMD_JOURNAL_COMPACT=1 \
	SYSTEMD_JOURNAL_KEYED_HASH=1 \
	SYSTEMD_JOURNAL_COMPRESS=zstd \
	"$remote" \
	--split-mode=none \
	--compress=yes \
	--output="$tmpdir/compact-keyed-zstd.journal" \
	"$export_file"

normalize_machine_id "$tmpdir/regular-uncompressed.journal"
normalize_machine_id "$tmpdir/compact-keyed-zstd.journal"

journalctl --verify --file="$tmpdir/regular-uncompressed.journal"
journalctl --verify --file="$tmpdir/compact-keyed-zstd.journal"

gzip -n "$tmpdir/regular-uncompressed.journal"
gzip -n "$tmpdir/compact-keyed-zstd.journal"
mv "$tmpdir/regular-uncompressed.journal.gz" "$output_dir/regular-uncompressed.journal.gz"
mv "$tmpdir/compact-keyed-zstd.journal.gz" "$output_dir/compact-keyed-zstd.journal.gz"

printf 'generated journal fixtures in %s\n' "$output_dir"
