#!/usr/bin/env bash
# Print a directory containing a GNU wc reference with the 9.5+ word semantics.
# Prefer an installed reference. Install only wc into a temporary directory on
# older CI images; do not replace the system coreutils or pin the host OS.
set -euo pipefail

for candidate in gnuwc gwc wc; do
    if ! executable=$(command -v "$candidate"); then
        continue
    fi
    version=$(LC_ALL=C "$executable" --version 2>/dev/null | head -1) || continue
    case "$version" in
        *'(GNU coreutils)'*) ;;
        *) continue ;;
    esac
    number=${version##* }
    major=${number%%.*}
    minor=${number#*.}
    minor=${minor%%.*}
    if [[ "$major" =~ ^[0-9]+$ && "$minor" =~ ^[0-9]+$ ]] && (( major > 9 || (major == 9 && minor >= 5) )); then
        dirname "$executable"
        exit 0
    fi
done

# The release checksum is published at:
# https://lists.gnu.org/archive/html/info-gnu/2025-04/msg00006.html
version=9.7
checksum=e8bb26ad0293f9b5a1fc43fb42ba970e312c66ce92c1b0b16713d7500db251bf
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/rshell-gnu-wc.XXXXXX")
archive="$build_dir/coreutils.tar.xz"
curl --fail --location --retry 3 --output "$archive" "https://ftp.gnu.org/gnu/coreutils/coreutils-$version.tar.xz" >&2
printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check >&2
tar -xJf "$archive" -C "$build_dir"
(
    cd "$build_dir/coreutils-$version"
    FORCE_UNSAFE_CONFIGURE=1 ./configure --disable-nls --disable-dependency-tracking >&2
    # The default target prepares gnulib headers before compiling utilities.
    make -j2 >&2
)
mkdir "$build_dir/bin"
cp "$build_dir/coreutils-$version/src/wc" "$build_dir/bin/gnuwc"
printf '%s\n' "$build_dir/bin"
