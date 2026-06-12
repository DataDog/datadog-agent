# Logs Agent Mutation Testing Summary

**Total packages processed:** 1

**By status:** ok=1

**Aggregate (ok packages):** 330 killed / 407 actionable (81.1%), 77 survived, 0 non-actionable

**Test-timeout kills:** 1 of 330 killed mutants (0%) hit the per-mutant test timeout rather than a real assertion. A high fraction in a package with slow tests may mean the cap is too tight; in concurrency-heavy code it usually just reflects deadlock-inducing mutations.

## Per package

| Target | Status | Score | Killed | Survived | Total | Timeout-kills | Duration(s) |
|--------|--------|------:|-------:|---------:|------:|--------------:|------------:|
| pkg/logs/internal/decoder/preprocessor | ok | 81.1% | 330 | 77 | 407 | 1 | 841.9 |
