#!/bin/sh

# Appends post-rename markers to each newly rotated file.
#
# The Log4j2 RollingFile writer closes the active file, renames it, and only
# then reopens the active path, so it never appends to a rotated inode. That
# makes the writer unable to produce the loss this suite cares about: a writer
# that keeps flushing to the old inode after the rename, past the point where
# the draining reader decides the rotated file is finished and stops reading it.
#
# This sidecar supplies exactly those late appends. It watches the log directory
# for a newly rotated file, then appends one marker line per configured age. The
# ages straddle every reader's drain window on purpose; see the marker constants
# in provisioner.go. Every append attempt is journalled so the test knows which
# markers exist and can require that the early one arrived and the late one did
# not.
#
# LOGWRITER_APPEND_DELAYS_MS=none disables the markers, for rotation modes that
# leave no renamed file to append to. With LOGWRITER_STREAMS=N, each stream
# directory svc-<i> is watched on its own, with the stream's run ID
# (<run ID>-svc-<i>) and a marker journal of its own.

set -eu

log_dir=${LOGWRITER_LOG_DIR:-/mnt/azure-files}
run_id=${LOGWRITER_RUN_ID:-unknown}
streams=${LOGWRITER_STREAMS:-0}
delays_ms=${LOGWRITER_APPEND_DELAYS_MS:-1500,45000}
poll_ms=${LOGWRITER_APPEND_POLL_MS:-200}

case $streams in
'' | *[!0-9]*)
    printf 'appender_fatal reason=invalid_streams streams=%s\n' "$streams" >&2
    exit 1
    ;;
esac

if [ "$delays_ms" = none ]; then
    printf 'appender_idle run_id=%s reason=no_markers\n' "$run_id"
    # The container must keep running.
    while :; do
        sleep 3600
    done
fi
for delay_ms in $(printf '%s' "$delays_ms" | tr ',' ' '); do
    case $delay_ms in
    '' | *[!0-9]*)
        printf 'appender_fatal reason=invalid_delay delays_ms=%s\n' "$delays_ms" >&2
        exit 1
        ;;
    esac
done

# Sub-second sleeps decide how closely a marker tracks its target age, so refuse
# to run with second resolution rather than emit markers that silently land late.
nap_mode=usleep
if sleep 0.01 2>/dev/null; then
    nap_mode=fractional
elif ! command -v usleep >/dev/null 2>&1; then
    printf 'appender_fatal reason=no_sub_second_sleep\n' >&2
    exit 1
fi

nap_ms() {
    milliseconds=$1
    if [ "$milliseconds" -le 0 ]; then
        return 0
    fi
    if [ "$nap_mode" = fractional ]; then
        sleep "$(printf '%s.%03d' $((milliseconds / 1000)) $((milliseconds % 1000)))"
    else
        usleep $((milliseconds * 1000))
    fi
}

journal() {
    printf '{"run_id":"%s","rotation":%s,"marker_age_ms":%s,"marker_id":"%s","rotated_file":"%s","appended_at":"%s","status":"%s"}\n' \
        "$stream_run_id" "$2" "$3" "$4" "$5" "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$1" >> "$journal_path"
}

# Appends every configured marker to one rotated path. It runs in the
# background, one job per rotation, so markers keep their ages when rotations
# come faster than the oldest marker age or in several directories at once.
append_markers() {
    rotated_path=$1
    rotated_name=$2
    rotation=$3

    elapsed_ms=0
    for delay_ms in $(printf '%s' "$delays_ms" | tr ',' ' '); do
        nap_ms $((delay_ms - elapsed_ms))
        elapsed_ms=$delay_ms
        marker_id="${stream_run_id}-r${rotation}-m${delay_ms}"

        # An append to a path that is gone would create a file of its own: a
        # rotated file that was compressed or deleted since gets no marker.
        if [ ! -f "$rotated_path" ]; then
            journal skipped "$rotation" "$delay_ms" "$marker_id" "$rotated_name"
            printf 'appender_marker_skipped file=%s rotation=%s marker_age_ms=%s marker_id=%s reason=file_gone\n' \
                "$rotated_name" "$rotation" "$delay_ms" "$marker_id"
            continue
        fi
        # The append targets the rotated path, so it lands on the file the
        # draining reader is still finishing (an open descriptor for a file
        # source, the same server FileId for the SMB source), not on the new
        # active file.
        if printf 'post_rotation_marker run_id=%s rotation=%s marker_age_ms=%s marker_id=%s rotated_file=%s\n' \
            "$stream_run_id" "$rotation" "$delay_ms" "$marker_id" "$rotated_name" >> "$rotated_path"; then
            journal appended "$rotation" "$delay_ms" "$marker_id" "$rotated_name"
            printf 'appender_marker_appended file=%s rotation=%s marker_age_ms=%s marker_id=%s\n' \
                "$rotated_name" "$rotation" "$delay_ms" "$marker_id"
        else
            journal failed "$rotation" "$delay_ms" "$marker_id" "$rotated_name"
            printf 'appender_marker_failed file=%s rotation=%s marker_age_ms=%s marker_id=%s\n' \
                "$rotated_name" "$rotation" "$delay_ms" "$marker_id" >&2
        fi
    done
}

# Watches one stream directory. Its variables are its own: each directory of
# a multi-stream layout is watched in a subshell.
watch_dir() {
    dir=$1
    stream_run_id=$2
    journal_path=$3
    seen=""
    rotation=0
    # The share is retained between runs, so rotated files that predate this
    # container are recorded as seen without markers. Appending to them would
    # produce markers whose real age is minutes, not milliseconds.
    primed=false

    printf 'appender_ready run_id=%s directory=%s delays_ms=%s poll_ms=%s nap_mode=%s\n' \
        "$stream_run_id" "$dir" "$delays_ms" "$poll_ms" "$nap_mode"

    while :; do
        for rotated_path in "$dir"/app.log.*; do
            if [ ! -f "$rotated_path" ]; then
                continue
            fi
            rotated_name=$(basename "$rotated_path")
            case $rotated_name in
            # A compressed rotated file is not one the writer rotated.
            *.gz) continue ;;
            esac
            if was_seen "$rotated_name"; then
                continue
            fi
            seen="$seen $rotated_name"

            if [ "$primed" = false ]; then
                printf 'appender_primed file=%s reason=predates_container\n' "$rotated_name"
                continue
            fi

            rotation=$((rotation + 1))
            printf 'appender_rotation_detected file=%s rotation=%s\n' "$rotated_name" "$rotation"
            append_markers "$rotated_path" "$rotated_name" "$rotation" &
        done
        primed=true
        nap_ms "$poll_ms"
    done
}

was_seen() {
    case " $seen " in
    *" $1 "*) return 0 ;;
    *) return 1 ;;
    esac
}

if [ "$streams" -eq 0 ]; then
    watch_dir "$log_dir" "$run_id" "${LOGWRITER_MARKER_JOURNAL_PATH:-${log_dir}/markers.jsonl}"
fi
index=1
while [ "$index" -le "$streams" ]; do
    watch_dir "${log_dir}/svc-${index}" "${run_id}-svc-${index}" "${log_dir}/svc-${index}/markers.jsonl" &
    index=$((index + 1))
done
wait
