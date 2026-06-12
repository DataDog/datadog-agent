# Mutation Testing: pkg/logs/internal/decoder/preprocessor

**Score: 78.9%** (321 killed / 407 actionable, 86 survived, 0 non-actionable, 407 total)

## Per-file

| File | Total | Killed | Survived | Other | Score |
|------|------:|-------:|---------:|------:|------:|
| `combining_aggregator.go` | 31 | 23 | 8 | 0 | 74.2% |
| `detecting_aggregator.go` | 21 | 14 | 7 | 0 | 66.7% |
| `go_stack_trace_parser.go` | 90 | 75 | 15 | 0 | 83.3% |
| `incremental_json_validator.go` | 11 | 11 | 0 | 0 | 100.0% |
| `json_aggregator.go` | 10 | 7 | 3 | 0 | 70.0% |
| `json_detector.go` | 1 | 1 | 0 | 0 | 100.0% |
| `pass_through_aggregator.go` | 2 | 1 | 1 | 0 | 50.0% |
| `pattern_table.go` | 25 | 19 | 6 | 0 | 76.0% |
| `preprocessor.go` | 9 | 2 | 7 | 0 | 22.2% |
| `regex_aggregator.go` | 14 | 5 | 9 | 0 | 35.7% |
| `sampler.go` | 33 | 31 | 2 | 0 | 93.9% |
| `stack_trace_aggregator.go` | 27 | 17 | 10 | 0 | 63.0% |
| `timestamp_detector.go` | 3 | 2 | 1 | 0 | 66.7% |
| `token_graph.go` | 27 | 25 | 2 | 0 | 92.6% |
| `tokenizer.go` | 87 | 76 | 11 | 0 | 87.4% |
| `user_samples.go` | 16 | 12 | 4 | 0 | 75.0% |

## Surviving mutants (86)

### `combining_aggregator.go`

- **Line 85** (CONDITIONALS_BOUNDARY) — `LIVED`
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

- **Line 77** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 77** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 81** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 81** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 90** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 111** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 122** (CONDITIONALS_NEGATION) — `LIVED`
### `regex_aggregator.go`

- **Line 107** (INCREMENT_DECREMENT) — `LIVED`
- **Line 120** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 140** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 161** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 161** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 162** (INVERT_NEGATIVES) — `LIVED`
- **Line 162** (ARITHMETIC_BASE) — `LIVED`
- **Line 181** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 181** (CONDITIONALS_NEGATION) — `LIVED`
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
- **Line 72** (CONDITIONALS_BOUNDARY) — `LIVED`
### `tokenizer.go`

- **Line 110** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 110** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 110** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 110** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 124** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 134** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 151** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 171** (ARITHMETIC_BASE) — `LIVED`
- **Line 171** (ARITHMETIC_BASE) — `LIVED`
- **Line 172** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 172** (CONDITIONALS_NEGATION) — `LIVED`
### `user_samples.go`

- **Line 46** (CONDITIONALS_NEGATION) — `LIVED`
- **Line 51** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 68** (CONDITIONALS_BOUNDARY) — `LIVED`
- **Line 68** (CONDITIONALS_BOUNDARY) — `LIVED`
