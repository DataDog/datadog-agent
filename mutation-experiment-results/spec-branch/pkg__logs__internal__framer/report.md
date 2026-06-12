# Mutation Testing: pkg/logs/internal/framer

**Score: 76.5%** (104 killed / 136 actionable, 32 survived, 0 non-actionable, 136 total)

## Per-file

| File | Total | Killed | Survived | Other | Score |
|------|------:|-------:|---------:|------:|------:|
| `docker_stream.go` | 28 | 22 | 6 | 0 | 78.6% |
| `framer.go` | 24 | 15 | 9 | 0 | 62.5% |
| `onebyte.go` | 10 | 9 | 1 | 0 | 90.0% |
| `syslog.go` | 64 | 49 | 15 | 0 | 76.6% |
| `twobyte.go` | 10 | 9 | 1 | 0 | 90.0% |

## Surviving mutants (32)

### `docker_stream.go`

- **Line 30** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 52** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 73** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 75** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 77** (INVERT_NEGATIVES) — `LIVED`
- **Line 77** (ARITHMETIC_BASE) — `LIVED`
### `framer.go`

- **Line 126** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 132** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 209** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 247** (ARITHMETIC_BASE) — `LIVED`
- **Line 247** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 247** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 264** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 272** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 272** (CONDITIONALS_NEGATION) — `LIVED`
### `onebyte.go`

- **Line 34** (CONDITIONALS_BOUNDARY) — `LIVED`
### `syslog.go`

- **Line 37** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 37** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 43** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 43** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 43** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 67** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 72** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 80** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 80** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 80** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 98** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 109** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 109** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 118** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 141** (CONDITIONALS_BOUNDARY) — `LIVED`
### `twobyte.go`

- **Line 50** (ARITHMETIC_BASE) — `LIVED`
