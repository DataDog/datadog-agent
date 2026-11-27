#!/usr/bin/env bash
# Inspects the outputs of a dd_strip_symbols tree split (see :test_tree_stripped):
#  - every ELF gets a .dbg twin at the mirrored path
#  - the shipped ELFs carry .gnu_debuglink pointers to their twins,
#  - non-ELF files pass through unchanged and get no .dbg twin.
set -euo pipefail

READELF="$1"
INPUT="$2"
STRIPPED="$3"
DEBUG="$4"

failed=0
ok() { echo "OK: $*"; }
bad() { echo "FAIL: $*"; failed=1; }

# Non-ELF file: present in the stripped tree, unchanged, no .dbg twin.
if cmp -s "$INPUT/lib/python/notelf.py" "$STRIPPED/lib/python/notelf.py"; then
    ok "non-ELF file copied unchanged"
else
    bad "non-ELF file differs in stripped tree"
fi
if [ ! -e "$DEBUG/lib/python/notelf.py.dbg" ]; then
    ok "non-ELF file has no .dbg twin"
else
    bad "non-ELF file unexpectedly has a .dbg twin"
fi

# The DWARF-carrying ELF at the tree root.
if "$READELF" -S "$DEBUG/test_binary.dbg" | grep "\.debug_info" > /dev/null; then
    ok "DWARF ELF .dbg twin carries .debug_info"
else
    bad "DWARF ELF .dbg twin lost its .debug_info"
fi
if ! sections=$("$READELF" -S "$STRIPPED/test_binary"); then
    bad "could not inspect stripped ELF"
elif grep -q '\.debug_info' <<< "$sections"; then
    bad "stripped ELF still carries .debug_info"
else
    ok "stripped ELF no longer carries .debug_info"
fi
if "$READELF" -p .gnu_debuglink "$STRIPPED/test_binary" 2>/dev/null | grep "test_binary.dbg" > /dev/null; then
    ok "stripped ELF has .gnu_debuglink to its .dbg twin"
else
    bad "stripped ELF misses .gnu_debuglink to its .dbg twin"
fi

# The debug tree contains only .dbg files.
list="$TEST_TMPDIR/debug_tree_files"
find -L "$DEBUG" -type f -print0 > "$list"
while IFS= read -r -d '' f; do
    case "$f" in
        *.dbg) ;;
        *) bad "unexpected non-.dbg file in debug tree: $f" ;;
    esac
done < "$list"

exit "$failed"
