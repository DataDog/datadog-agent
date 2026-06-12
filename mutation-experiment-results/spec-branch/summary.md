# Logs Agent Mutation Testing Summary

**Total packages processed:** 2

**By status:** ok=2

**Aggregate (ok packages):** 436 killed / 543 actionable (80.3%), 107 survived, 0 non-actionable

**Test-timeout kills:** 14 of 436 killed mutants (3%) hit the per-mutant test timeout rather than a real assertion. A high fraction in a package with slow tests may mean the cap is too tight; in concurrency-heavy code it usually just reflects deadlock-inducing mutations.

## Per package

| Target | Status | Score | Killed | Survived | Total | Timeout-kills | Duration(s) |
|--------|--------|------:|-------:|---------:|------:|--------------:|------------:|
| pkg/logs/internal/decoder/preprocessor | ok | 81.6% | 332 | 75 | 407 | 1 | 1204.6 |
| pkg/logs/internal/framer | ok | 76.5% | 104 | 32 | 136 | 13 | 1762.6 |
