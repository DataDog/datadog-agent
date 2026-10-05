#!/bin/sh

set -eu

log_dir=${LOGWRITER_LOG_DIR:-/mnt/azure-files}
ledger_path=${LOGWRITER_LEDGER_PATH:-${log_dir}/ledger.jsonl}
seen_dir=${LOGWRITER_LEDGER_SEEN_DIR:-${log_dir}/.ledger-seen}
poll_seconds=${LOGWRITER_LEDGER_POLL_SECONDS:-1}
# Prints Go's hash/crc64 ISO checksum of a file's first 2048 bytes or first
# line. The Java image runs its Crc64 class; the stock image runs logwriter.py,
# which prints the same value.
crc64_command=${LOGWRITER_CRC64_COMMAND:-java -cp /app/classes com.datadoghq.e2e.logwriter.Crc64}

mkdir -p "$seen_dir"

crc64() {
    # shellcheck disable=SC2086 # the command is split into its words on purpose
    $crc64_command "$@"
}

record_file() {
    rotated_path=$1
    rotated_name=$(basename "$rotated_path")
    seen_path=${seen_dir}/${rotated_name}
    if [ -e "$seen_path" ]; then
        return
    fi

    discovered_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
    bytes=$(wc -c < "$rotated_path" | tr -d ' ')
    lines=$(wc -l < "$rotated_path" | tr -d ' ')
    first_2048_sha256=$(dd if="$rotated_path" bs=2048 count=1 2>/dev/null | sha256sum | awk '{print $1}')
    first_line_sha256=$(sed -n '1{s/\r$//;p;}' "$rotated_path" | tr -d '\n' | sha256sum | awk '{print $1}')
    first_2048_crc64=$(crc64 bytes "$rotated_path")
    first_line_crc64=$(crc64 line "$rotated_path")
    run_id=$(sed -n 's/.*run_id=\([^ |]*\).*/\1/p' "$rotated_path" | head -n 1)
    period=$(sed -n 's/.*period=\([^ |]*\).*/\1/p' "$rotated_path" | head -n 1)
    target_bytes=$(sed -n 's/.*target_bytes=\([0-9][0-9]*\).*/\1/p' "$rotated_path" | head -n 1)
    first_sequence=$(grep -o 'sequence=[0-9][0-9]*' "$rotated_path" | head -n 1 | cut -d= -f2 || true)
    last_sequence=$(grep -o 'sequence=[0-9][0-9]*' "$rotated_path" | tail -n 1 | cut -d= -f2 || true)
    observed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

    if [ -z "$run_id" ] || [ -z "$period" ] || [ -z "$target_bytes" ] || [ -z "$first_sequence" ] || [ -z "$last_sequence" ]; then
        printf 'ledger_skip file=%s reason=missing_markers\n' "$rotated_name" >&2
        return
    fi

    printf '{"run_id":"%s","period":"%s","file":"%s","target_bytes":%s,"bytes":%s,"first_2048_sha256":"%s","first_2048_crc64":"%s","first_line_sha256":"%s","first_line_crc64":"%s","first_sequence":%s,"last_sequence":%s,"line_count":%s,"discovered_at":"%s","observed_at":"%s"}\n' \
        "$run_id" \
        "$period" \
        "$rotated_name" \
        "$target_bytes" \
        "$bytes" \
        "$first_2048_sha256" \
        "$first_2048_crc64" \
        "$first_line_sha256" \
        "$first_line_crc64" \
        "$first_sequence" \
        "$last_sequence" \
        "$lines" \
        "$discovered_at" \
        "$observed_at" >> "$ledger_path"
    : > "$seen_path"
    printf 'ledger_recorded file=%s bytes=%s first_sequence=%s last_sequence=%s\n' \
        "$rotated_name" "$bytes" "$first_sequence" "$last_sequence"
}

while :; do
    found=false
    for rotated_path in "$log_dir"/app.log.*; do
        if [ ! -f "$rotated_path" ]; then
            continue
        fi
        found=true
        record_file "$rotated_path"
    done
    if [ "$found" = false ]; then
        printf 'ledger_waiting directory=%s\n' "$log_dir"
    fi
    sleep "$poll_seconds"
done
