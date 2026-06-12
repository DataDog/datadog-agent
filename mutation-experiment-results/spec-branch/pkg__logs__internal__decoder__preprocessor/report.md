# Mutation Testing: pkg/logs/internal/decoder/preprocessor

**Score: 81.6%** (332 killed / 407 actionable, 75 survived, 0 non-actionable, 407 total)

## Per-file

| File | Total | Killed | Survived | Other | Score |
|------|------:|-------:|---------:|------:|------:|
| `combining_aggregator.go` | 31 | 24 | 7 | 0 | 77.4% |
| `detecting_aggregator.go` | 21 | 14 | 7 | 0 | 66.7% |
| `go_stack_trace_parser.go` | 90 | 75 | 15 | 0 | 83.3% |
| `incremental_json_validator.go` | 11 | 11 | 0 | 0 | 100.0% |
| `json_aggregator.go` | 10 | 7 | 3 | 0 | 70.0% |
| `json_detector.go` | 1 | 1 | 0 | 0 | 100.0% |
| `pass_through_aggregator.go` | 2 | 1 | 1 | 0 | 50.0% |
| `pattern_table.go` | 25 | 19 | 6 | 0 | 76.0% |
| `preprocessor.go` | 9 | 6 | 3 | 0 | 66.7% |
| `regex_aggregator.go` | 14 | 9 | 5 | 0 | 64.3% |
| `sampler.go` | 33 | 31 | 2 | 0 | 93.9% |
| `stack_trace_aggregator.go` | 27 | 17 | 10 | 0 | 63.0% |
| `timestamp_detector.go` | 3 | 2 | 1 | 0 | 66.7% |
| `token_graph.go` | 27 | 26 | 1 | 0 | 96.3% |
| `tokenizer.go` | 87 | 77 | 10 | 0 | 88.5% |
| `user_samples.go` | 16 | 12 | 4 | 0 | 75.0% |

## Surviving mutants (75)

### `combining_aggregator.go`

- **Line 85** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 136** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 184** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 196** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 196** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 197** (INVERT_NEGATIVES) — `LIVED`
- **Line 197** (ARITHMETIC_BASE) — `LIVED`
### `detecting_aggregator.go`

- **Line 144** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 167** (ARITHMETIC_BASE) — `LIVED`
- **Line 167** (ARITHMETIC_BASE) — `LIVED`
- **Line 167** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 172** (ARITHMETIC_BASE) — `LIVED`
- **Line 182** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 198** (CONDITIONALS_BOUNDARY) — `LIVED`
### `go_stack_trace_parser.go`

- **Line 243** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 245** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 245** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 264** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 264** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 264** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 264** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 264** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 270** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 323** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 330** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 348** (ARITHMETIC_BASE) — `LIVED`
- **Line 348** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 356** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 356** (CONDITIONALS_BOUNDARY) — `LIVED`
### `json_aggregator.go`

- **Line 62** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 70** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 98** (CONDITIONALS_BOUNDARY) — `LIVED`
### `pass_through_aggregator.go`

- **Line 35** (CONDITIONALS_BOUNDARY) — `LIVED`
### `pattern_table.go`

- **Line 85** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 101** (INVERT_NEGATIVES) — `LIVED`
- **Line 101** (ARITHMETIC_BASE) — `LIVED`
- **Line 107** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 119** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 149** (CONDITIONALS_NEGATION) — `LIVED`
### `preprocessor.go`

- **Line 81** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 81** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 122** (CONDITIONALS_NEGATION) — `LIVED`
### `regex_aggregator.go`

- **Line 120** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 161** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 161** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 162** (INVERT_NEGATIVES) — `LIVED`
- **Line 162** (ARITHMETIC_BASE) — `LIVED`
### `sampler.go`

- **Line 233** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 265** (CONDITIONALS_BOUNDARY) — `LIVED`
### `stack_trace_aggregator.go`

- **Line 90** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 103** (ARITHMETIC_BASE) — `LIVED`
- **Line 103** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 169** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 169** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 175** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 180** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 189** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 189** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 287** (CONDITIONALS_NEGATION) — `LIVED`
### `timestamp_detector.go`

- **Line 117** (CONDITIONALS_BOUNDARY) — `LIVED`
### `token_graph.go`

- **Line 50** (CONDITIONALS_BOUNDARY) — `LIVED`
### `tokenizer.go`

- **Line 110** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 110** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 110** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 110** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 124** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 134** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 171** (ARITHMETIC_BASE) — `LIVED`
- **Line 171** (ARITHMETIC_BASE) — `LIVED`
- **Line 172** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 172** (CONDITIONALS_NEGATION) — `LIVED`
### `user_samples.go`

- **Line 46** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 51** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 68** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 68** (CONDITIONALS_BOUNDARY) — `LIVED`
