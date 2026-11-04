#!/usr/bin/env bash
# Checks that the native build of libjemalloc matches the configure_make one.
# Byte equality is out of reach (build ids, data layout), so this compares the
# dynamic section, the exported symbols and the code of every function.
set -euo pipefail

ref=$1
native=$2
diff_file=${TEST_TMPDIR:-$(mktemp -d)}/parity.diff
status=0

soname() { readelf -d "$1" | awk "/SONAME/ {print \$NF}"; }
needed() { readelf -d "$1" | awk "/NEEDED/ {print \$NF}" | sort; }
exports() { nm -D --defined-only "$1" | awk "{print \$2, \$3}" | sort; }
# Opcodes only: operands embed addresses that move with the data layout.
functions() {
    objdump -d --no-show-raw-insn -j .text "$1" | awk -F"\t" "
        /^[0-9a-f]+ <.*>:\$/ { fn = \$0; sub(/^[0-9a-f]+ /, \"\", fn); next }
        NF >= 2 { split(\$2, insn, \" \"); ops[fn] = ops[fn] insn[1] \" \" }
        END { for (fn in ops) print fn \"\t\" ops[fn] }" | sort
}
text_section() { objcopy -O binary --only-section=.text "$1" /dev/stdout | sha256sum; }

compare() {
    local what=$1
    if diff "$2" "$3" >"$diff_file"; then
        echo "PASS $what"
    else
        echo "FAIL $what"
        head -20 "$diff_file"
        status=1
    fi
}

compare SONAME <(soname "$ref") <(soname "$native")
compare NEEDED <(needed "$ref") <(needed "$native")
compare "exported symbols" <(exports "$ref") <(exports "$native")
compare "per-function code" <(functions "$ref") <(functions "$native")
if [[ "$(text_section "$ref")" == "$(text_section "$native")" ]]; then
    echo "INFO .text is byte-identical"
else
    echo "INFO .text differs at byte level"
fi
exit "$status"
