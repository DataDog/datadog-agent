The `header.tar.*` fixtures contain a single `file.h` with the text `header`.
They were generated using Python's standard tarfile, lzma and bz2 modules.

The six `payload-test-*.rpm` fixtures are from
[sassoftware/go-rpmutils v0.4.0](https://github.com/sassoftware/go-rpmutils/tree/v0.4.0/testdata).
Their public-domain `payload-test.spec` packages `/usr/share/payload-test.txt`
with the text `Some data\n`, using uncompressed, gzip, bzip2, LZMA, xz and zstd
payloads. These are test inputs only and are not shipped with the Agent.
