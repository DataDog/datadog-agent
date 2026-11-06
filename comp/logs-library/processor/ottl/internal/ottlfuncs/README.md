# Agent OTTL function subset

These function implementations are copied from
`github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v1.3.1`,
under `ottlfuncs/`. Their OpenTelemetry copyright and Apache-2.0 SPDX headers
are preserved; the upstream license is included in `LICENSE`. The canonical package import comment is removed. `func_is_list.go` and
`func_len.go` additionally omit Collector log, metric, and trace slice types:
the Agent context exposes JSON/body values and cannot return those wrappers.
Native JSON and pcommon container handling is retained.

The Agent registers 13 converters and three masking editors. Importing even one
factory from the upstream `ottlfuncs` package brings in initialization code and
data from unrelated XML, grok, and user-agent parsers. A reduced registry alone
does not remove those dependencies. This package copies only the required
factories and `regex_helpers.go`, leaving the OTTL parser and typed getters in
the upstream module.

## Updating

When changing the OTTL dependency version:

1. Compare each `func_*.go` and `regex_helpers.go` here with the corresponding
   upstream file at the selected module version. Recopy upstream changes,
   removing the canonical package import comment and retaining the documented
   Agent-context adaptations in `IsList` and `Len`. Update `LICENSE`.
2. Update the version in this file.
3. Run `TestOTTLFunctionsMatchUpstream` with the logs runtime package's normal
   tests. It compares compilation, evaluation, errors, and masking output with
   the upstream factories across the Agent's JSON/body context.
4. Run Gazelle for `comp/logs-library/processor/ottl` and inspect the production dependency
   graph and binary-size delta. The upstream `ottlfuncs` import is intentional
   only in tests and benchmarks; do not reintroduce it in production.

Keep function bodies aligned with upstream instead of independently optimizing
or extending them. Configuration examples and the parser's supported function
registry remain the public contract.
