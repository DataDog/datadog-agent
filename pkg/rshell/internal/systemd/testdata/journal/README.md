# Journal fixtures

These fixtures contain four synthetic records with fixed machine, boot, and
timestamps. They contain no logs or identifiers copied from a real host. The
generation script also replaces the journal header's generator machine ID with
the fixed synthetic machine ID before verification.

The two committed golden files were generated and verified with systemd 255.4
on Ubuntu 24.04. `regular-uncompressed.journal.gz` exercises the original
regular layout. `compact-keyed-zstd.journal.gz` exercises compact offsets,
keyed hashes, and a Zstandard-compressed DATA object. Gzip only keeps the sparse
8 MiB journal files small in Git; tests decompress them in memory or into a
per-test temporary directory. Test runs do not write generated files here.

The golden files remain committed so tests can validate files emitted by real
systemd on hosts and CI workers that do not provide `systemd-journal-remote`.
`fixture.export` is their synthetic source rather than generated output.

On a Linux system with `systemd-journal-remote`, generate a fresh corpus in an
explicit temporary output directory and test it with:

```sh
output_dir=$(mktemp -d)
./generate.sh "$output_dir"
RSHELL_JOURNAL_FIXTURE_DIR="$output_dir" \
  go test ../.. \
  -run 'TestRealJournalFixtures|TestJournalFixtureMatchesJournalctl|TestReadJournalUsesPureGoFixtureBackend'
```

Journal file and sequence IDs are randomized by systemd, so regeneration is
semantically deterministic but does not reproduce byte-identical files. The
generator never replaces the committed golden corpus unless this directory is
passed explicitly as its output directory.
