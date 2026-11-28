# extract_symbols

Exports the symbol ELF the host profiler's symbol uploader would upload, so it
can be inspected or checked. Linux only.

```sh
bazel run //tools/host-profiler/extract_symbols -- [-stream] INPUT EXPORT
```

- Without `-stream`, it runs objcopy like the uploader's default path. objcopy
  gets the `CAP_SYS_PTRACE` and `CAP_DAC_READ_SEARCH` ambient capabilities, as
  in the profiler, so run it as root or grant them.
- With `-stream`, it uses elfstream, like `symbol_uploader.stream_symbols`.

The export is not compressed, so ELF tools read it directly.

## Checking an export

```sh
tools/host-profiler/extract_symbols/verify.sh INPUT EXPORT
```

`verify.sh` prints `PASS INPUT` or `FAIL INPUT: reason` and exits 1 on failure.
It compares the export with its input using standard tools:

- readelf reports no new warning for headers, symbols and notes;
- symbols and notes match (`readelf --syms --dyn-syms`, `readelf -n`), with
  owning sections compared by name, apart from the Go markers the export adds
  or moves and the omitted symbol versions;
- every `.debug_*` section has the same bytes, compressed or not;
- every section with an address keeps it and its size, so addresses map to
  the same symbols and DWARF;
- for Go binaries with a `runtime.text` symbol, 1,000 addresses spread over
  `.text` resolve the same with `go tool addr2line`.

It needs binutils, and `go` for the Go check.

Known difference: objcopy exports of Go binaries fail the symbol check, because
removing and re-adding `.gopclntab` drops `runtime.pclntab`,
`runtime.findfunctab` and `runtime.epclntab`. The backend reads `.gopclntab` by
name, so uploads are unaffected.

To check every debug file under a directory, eight at a time:

```sh
find /usr/lib/debug -type f -name '*.debug' | xargs -P 8 -I{} sh -c \
  'x=$(mktemp) && extract_symbols -stream "$1" "$x" && verify.sh "$1" "$x"; rm -f "$x"' _ {}
```
