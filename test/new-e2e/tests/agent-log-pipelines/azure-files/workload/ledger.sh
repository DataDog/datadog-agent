#!/bin/sh

# Records every file the writer completed in ledger.jsonl, once.
#
# LOGWRITER_LEDGER_SOURCE selects where a completed file is learnt from:
#   - scan (the default, which the Java writer image runs): each rotated file
#     app.log.* is recorded from its content;
#   - journal: each line the writer appended to its journal (periods.jsonl)
#     before it renamed, copied, truncated, compressed or deleted a file is
#     recorded as it is, with what the share holds for that file when the
#     ledger looks. The stock Python writer journals, so its ledger also
#     covers files that are already gone or compressed.
#
# With LOGWRITER_STREAMS=N the writer writes N streams, svc-1/app.log to
# svc-<N>/app.log, and every stream directory gets its own ledger.

set -eu

log_dir=${LOGWRITER_LOG_DIR:-/mnt/azure-files}
ledger_source=${LOGWRITER_LEDGER_SOURCE:-scan}
streams=${LOGWRITER_STREAMS:-0}
journal_name=${LOGWRITER_JOURNAL_NAME:-periods.jsonl}
poll_seconds=${LOGWRITER_LEDGER_POLL_SECONDS:-1}
# Prints Go's hash/crc64 ISO checksum of a file's first 2048 bytes or first
# line. The Java image runs its Crc64 class; the stock image runs logwriter.py,
# which prints the same value.
crc64_command=${LOGWRITER_CRC64_COMMAND:-java -cp /app/classes com.datadoghq.e2e.logwriter.Crc64}

case $ledger_source in
scan | journal) ;;
*)
    printf 'ledger_fatal reason=unknown_source source=%s\n' "$ledger_source" >&2
    exit 1
    ;;
esac
case $streams in
'' | *[!0-9]*)
    printf 'ledger_fatal reason=invalid_streams streams=%s\n' "$streams" >&2
    exit 1
    ;;
esac

crc64() {
    # shellcheck disable=SC2086 # the command is split into its words on purpose
    $crc64_command "$@"
}

# The stream directories, one per line.
stream_dirs() {
    if [ "$streams" -eq 0 ]; then
        printf '%s\n' "$log_dir"
        return
    fi
    index=1
    while [ "$index" -le "$streams" ]; do
        printf '%s/svc-%s\n' "$log_dir" "$index"
        index=$((index + 1))
    done
}

# Sets the ledger and seen paths of a stream directory. The overrides only
# apply to the single stream at the root of the log directory.
use_dir() {
    dir=$1
    if [ "$streams" -eq 0 ]; then
        ledger_path=${LOGWRITER_LEDGER_PATH:-${dir}/ledger.jsonl}
        seen_dir=${LOGWRITER_LEDGER_SEEN_DIR:-${dir}/.ledger-seen}
    else
        ledger_path=${dir}/ledger.jsonl
        seen_dir=${dir}/.ledger-seen
    fi
}

mark_seen() {
    if [ ! -d "$seen_dir" ]; then
        mkdir -p "$seen_dir"
    fi
    : > "$1"
}

# Prints the size of a file, or -1 when it cannot be read.
file_bytes() {
    bytes=$(wc -c 2>/dev/null < "$1" | tr -d ' ')
    if [ -z "$bytes" ]; then
        bytes=-1
    fi
    printf '%s' "$bytes"
}

# Prints the value of a string field of one journal line. The writer writes
# every field on one line, with no escaped characters in the values read here.
json_string() {
    printf '%s\n' "$1" | sed -n "s/.*\"$2\":\"\\([^\"]*\\)\".*/\\1/p"
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
    mark_seen "$seen_path"
    printf 'ledger_recorded file=%s bytes=%s first_sequence=%s last_sequence=%s\n' \
        "$rotated_name" "$bytes" "$first_sequence" "$last_sequence"
}

# Records one journal line, keyed by its period, with what the share holds for
# its file now: "file" (the rotated or copied file), "archive" (only its .gz),
# "deleted" (the writer deleted it, by design), or "missing".
record_journal_line() {
    line=$1
    period=$(json_string "$line" period)
    if [ -z "$period" ]; then
        printf 'ledger_skip journal=%s reason=no_period\n' "$journal_path" >&2
        return
    fi
    seen_path=${seen_dir}/period-${period}
    if [ -e "$seen_path" ]; then
        return
    fi

    discovered_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
    file=$(json_string "$line" file)
    archive=$(json_string "$line" archive)
    disposition=$(json_string "$line" disposition)
    observed=missing
    observed_bytes=-1
    if [ "$disposition" = deleted ]; then
        observed=deleted
    elif [ -n "$file" ] && [ -f "${dir}/${file}" ]; then
        observed="file"
        observed_bytes=$(file_bytes "${dir}/${file}")
    elif [ -n "$archive" ] && [ -f "${dir}/${archive}" ]; then
        observed=archive
        observed_bytes=$(file_bytes "${dir}/${archive}")
    fi
    observed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

    # The journal line is one JSON object: append the observation to it.
    printf '%s,"discovered_at":"%s","observed_at":"%s","observed":"%s","observed_bytes":%s}\n' \
        "${line%?}" "$discovered_at" "$observed_at" "$observed" "$observed_bytes" >> "$ledger_path"
    mark_seen "$seen_path"
    printf 'ledger_recorded file=%s period=%s disposition=%s observed=%s\n' \
        "$file" "$period" "$disposition" "$observed"
}

record_journal() {
    journal_path=${dir}/${journal_name}
    # read fails on a last line without its newline, which the writer is
    # still writing: it is recorded on the next poll.
    while IFS= read -r line; do
        case $line in
        '{'*'}') record_journal_line "$line" ;;
        '') ;;
        *) printf 'ledger_skip journal=%s reason=not_an_object\n' "$journal_path" >&2 ;;
        esac
    done < "$journal_path"
}

if [ "$streams" -eq 0 ]; then
    use_dir "$log_dir"
    mkdir -p "$seen_dir"
fi

while :; do
    found=false
    for dir in $(stream_dirs); do
        use_dir "$dir"
        if [ "$ledger_source" = journal ]; then
            if [ -f "${dir}/${journal_name}" ]; then
                found=true
                record_journal
            fi
            continue
        fi
        for rotated_path in "$dir"/app.log.*; do
            if [ ! -f "$rotated_path" ]; then
                continue
            fi
            found=true
            record_file "$rotated_path"
        done
    done
    if [ "$found" = false ]; then
        printf 'ledger_waiting directory=%s source=%s\n' "$log_dir" "$ledger_source"
    fi
    sleep "$poll_seconds"
done
