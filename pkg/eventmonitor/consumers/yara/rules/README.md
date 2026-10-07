# Embedded YARA rules — provenance and licensing

The `*.yar` files in this directory are the default rule set of the system-probe YARA exec
scanner. They are embedded into the Agent binary at build time (see `../embedded_rules.go`,
`//go:embed all:rules`) and compiled by the YARA engine at startup.

## Source

These are community-contributed rules collected from **YARAify** (https://yaraify.abuse.ch/),
a service operated by **abuse.ch**. They are third-party, community-authored detection rules,
not written by Datadog.

## Licensing

Licenses are **per-rule and set by the upstream author**. YARAify aggregates rules from many
contributors under mixed licenses; there is no single license covering the whole set. Each rule's
license (and author/attribution, where present) is whatever the upstream author assigned to it.
This directory is distributed as a convenience default; consult the upstream rule and author for
the terms that apply to any individual rule.

Because these files are third-party data rather than Go dependencies, they are not listed in the
repository's auto-generated `LICENSE-3rdparty.csv` (which tracks Go modules only).

## Curation

Only rules that compile cleanly against the real libyara engine are embedded: the loader compiles
every rule file together and a single failing file would disable the whole scanner. Rules that
import YARA modules the Agent does not build (e.g. `pe`, `dotnet`, `hash`, `magic`, `macho`,
`dex`, `cuckoo`, `androguard`, `console`) are excluded, and files that fail to compile individually
are dropped. The vetting helper lives in `../rules_compilecheck_test.go`
(`TestEmbeddedRulesCompileIndividually`, gated on the `YARA_RULES_DIR` env var); the
always-on `TestEmbeddedRulesCompileTogether` guards that the embedded set keeps compiling.
