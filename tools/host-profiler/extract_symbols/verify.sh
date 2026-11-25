#!/usr/bin/env bash
# verify.sh INPUT EXPORT: checks that EXPORT, an ELF that extract_symbols built
# from INPUT, reads and symbolizes like INPUT, using standard ELF tools.
# Needs binutils (readelf); uses `go tool addr2line` for Go binaries when go is
# available.
set -euo pipefail

if [ $# -ne 2 ]; then
	echo "usage: $0 INPUT EXPORT" >&2
	exit 2
fi
input=$1
export=$2
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
trap 'echo "FAIL $input: verify.sh line $LINENO failed"' ERR

fail() {
	echo "FAIL $input: $1"
	[ $# -lt 3 ] || diff "$2" "$3" | head -20
	exit 1
}

# readelf output for f, without its messages.
readelf_of() {
	local f=$1
	shift
	readelf -W "$@" "$f" 2>/dev/null
}

# readelf messages about f's headers, symbols and notes, without its name.
# objcopy keeps a PT_INTERP whose path it omits, which readelf reports.
readelf_warnings() {
	readelf -W -h -S -s -n "$1" 2>&1 >/dev/null |
		awk -v f="$1" '{ while ((i = index($0, f)) > 0) $0 = substr($0, 1, i - 1) substr($0, i + length(f)); print }' |
		{ grep -v 'Unable to find program interpreter name' || true; } | sort -u
}

# 1. The export parses without new warnings.
readelf_warnings "$input" >"$work/in.warn"
readelf_warnings "$export" >"$work/out.warn"
comm -13 "$work/in.warn" "$work/out.warn" >"$work/new.warn"
[ ! -s "$work/new.warn" ] || fail "readelf warnings: $(head -3 "$work/new.warn")"

# 2. Same symbols in every table the export keeps; .symtab is always kept, and
# .dynsym only when it is the best symbol source. Owning sections compare by
# name, as objcopy renumbers sections. Version suffixes are dropped (version
# tables are omitted), and so are the Go markers the export adds or moves and
# section symbols, which objcopy adds to a new .symtab.
symbols() {
	{
		readelf_of "$1" -S
		readelf_of "$1" --syms --dyn-syms
	} | awk '
		/^ *\[ *[0-9]+\]/ {
			line = $0; sub(/^ *\[ */, "", line); split(line, f, /\] +/); split(f[2], g, " ")
			section[f[1] + 0] = g[1]
			next
		}
		/^Symbol table/ { table = $3; gsub(/\047/, "", table); next }
		$1 ~ /^[0-9]+:$/ && $1 != "0:" && $4 != "SECTION" {
			name = $8; sub(/@.*/, "", name)
			if (name == "go:func.*" || name == "go.func.*" || name == "runtime.text") next
			owner = ($7 ~ /^[0-9]+$/) ? section[$7 + 0] : $7
			print table, $2, $3, $4, $5, owner, name
		}' | sort
}
symbols "$export" >"$work/out.syms"
symbols "$input" | awk 'FILENAME == ARGV[1] { kept[$1] = 1; next } kept[$1] || $1 == ".symtab"' "$work/out.syms" - >"$work/in.syms"
cmp -s "$work/in.syms" "$work/out.syms" || fail "symbols differ" "$work/in.syms" "$work/out.syms"

# 3. Same build IDs and other notes.
readelf_of "$input" -n >"$work/in.notes"
readelf_of "$export" -n >"$work/out.notes"
cmp -s "$work/in.notes" "$work/out.notes" || fail "notes differ" "$work/in.notes" "$work/out.notes"

# section_bytes f name: f's bytes for section name, as stored (compressed or not).
section_bytes() {
	readelf_of "$1" -S | awk -v name="$2" '
		{ sub(/^ *\[ *[0-9]+\] */, "") }
		$1 == name && $2 != "NOBITS" { print $4, $5; exit }' | {
		read -r offset size || exit 0
		tail -c +$((16#$offset + 1)) "$1" | head -c $((16#$size))
	}
}

# 4. Same DWARF bytes: both exporters copy debug sections unchanged.
sections=$(readelf_of "$input" -S)
for name in $(awk '{ sub(/^ *\[ *[0-9]+\] */, "") } $1 ~ /^\.z?debug_/ && $2 != "NOBITS" { print $1 }' <<<"$sections"); do
	[ "$(section_bytes "$input" "$name" | sha256sum)" = "$(section_bytes "$export" "$name" | sha256sum)" ] ||
		fail "$name differs"
done

# 5. Sections with an address keep it and their size, compared by name, so
# addresses map to the same symbols and DWARF. Sections without one, such as
# the name and symbol tables, can grow.
section_layout() {
	readelf_of "$1" -S | awk 'sub(/^ *\[ *[0-9]+\] */, "") && $3 ~ /^[0-9a-f]+$/ && $3 !~ /^0+$/ { print $1, $3, $5 }' | sort -u
}
section_layout "$export" >"$work/out.layout"
section_layout "$input" | awk 'FILENAME == ARGV[1] { kept[$1] = 1; next } kept[$1]' "$work/out.layout" - >"$work/in.layout"
awk 'FILENAME == ARGV[1] { want[$1] = 1; next } want[$1]' "$work/in.layout" "$work/out.layout" >"$work/out.kept"
cmp -s "$work/in.layout" "$work/out.kept" || fail "section addresses differ" "$work/in.layout" "$work/out.kept"

# 6. For Go binaries, 1,000 addresses spread over .text resolve the same with
# go tool addr2line, which reads the Go tables. Without a runtime.text symbol,
# go tool addr2line assumes Go code starts at .text, which is wrong for cgo
# binaries; the export adds the right one, so the input is no reference then.
read -r text_addr text_size < <(awk '{ sub(/^ *\[ *[0-9]+\] */, "") } $1 == ".text" { print $3, $5 }' <<<"$sections") || true
if [ -n "${text_addr:-}" ] && grep -qE ' \.(data\.rel\.ro\.)?gopclntab ' <<<"$sections" && command -v go >/dev/null &&
	readelf_of "$input" -s | awk '$8 == "runtime.text" { found = 1 } END { exit !found }'; then
	awk -v start=$((16#$text_addr)) -v size=$((16#$text_size)) \
		'BEGIN { for (i = 0; i < 1000; i++) printf "0x%x\n", start + int(size * i / 1000) }' >"$work/addrs"
	go tool addr2line "$input" <"$work/addrs" >"$work/in.go" 2>/dev/null
	go tool addr2line "$export" <"$work/addrs" >"$work/out.go" 2>/dev/null
	cmp -s "$work/in.go" "$work/out.go" || fail "go tool addr2line differs" "$work/in.go" "$work/out.go"
fi

echo "PASS $input"
