# Tokenizer versus clustering factorial

## Question

Where does the semantic pattern extractor's overhead and accuracy come from: its rich tokenizer, or its type-aware cluster matcher?

The current semantic matcher is more precisely a **type-aware matcher with an evolving display pattern**. When two messages merge, differing positions are marked as wildcards for display, but future matching still compares against the first message's retained token values.

## Controlled arms

All semantic-token controls use the same rich tokenizer and retain each token's `(type, raw value)` pair.

| Arm | Matching behavior |
|---|---|
| Semantic value exact | Equal token count and exact `(type, value)` equality at every position; no clustering |
| Semantic value adaptive 0.5 | Adaptive-sampler algorithm: compare the shared prefix length and require 50% exact `(type, value)` equality against a fixed representative |
| Semantic value equal-length 0.5 | Same fixed-representative comparison, but reject different token counts |
| Current semantic type-aware 0.5 | Require equal token counts, type compatibility at every position, and 50% exact raw-value equality; search signature buckets and mark differing display positions as wildcards |
| Logs-token fuzzy 0.5 | Reference: compact Logs tokenizer plus adaptive-sampler positional matching |

All accuracy cells use the same 12 logs-only scenarios, five-log emission threshold, five-minute detector baseline with noisy-series muting, `time_cluster`, and Gaussian temporal F1 scoring with sigma 30 seconds. JSON numeric and connection-error extraction are disabled. CUSUM is excluded; RRCF is the no-anomaly density control.

## Accuracy

| Extractor | Holt residual | ScanMW | ScanWelch | Tukey biweight | BOCPD | Mean over five |
|---|---:|---:|---:|---:|---:|---:|
| Semantic value exact | 0.0000 | 0.0000 | 0.0000 | 0.0000 | 0.0000 | 0.0000 |
| Logs-token fuzzy 0.5 | 0.2188 | 0.2580 | **0.2058** | 0.0881 | 0.1341 | 0.1810 |
| Semantic value adaptive 0.5 | 0.3587 | 0.2420 | 0.1609 | 0.1442 | 0.1730 | 0.2158 |
| Semantic value equal-length 0.5 | **0.3593** | 0.2420 | 0.1609 | 0.1427 | 0.1859 | 0.2181 |
| Current semantic type-aware 0.5 | 0.2505 | **0.3205** | 0.1609 | **0.1781** | **0.1903** | **0.2201** |

Exact semantic values fragment almost completely and produce zero F1 with every detector. Clustering is necessary.

With tokenization held constant, the simple equal-length matcher reaches mean F1 0.2181 versus 0.2201 for the current type-aware matcher: an absolute difference of 0.0019, or less than 1% relative. The type-aware rules therefore add little average accuracy on these scenarios, although they redistribute accuracy across detectors and incidents. For example, the simple matcher is much stronger with Holt, while the current matcher is stronger with ScanMW and Tukey.

Replacing the rich tokenizer with the Logs tokenizer while keeping a simple fuzzy matcher lowers mean F1 from 0.2181 to 0.1810. Within this experiment, most of the remaining accuracy difference follows the token representation rather than the type-aware clustering rules.

## Output density

| Extractor | Derived samples | Emitted series | Samples per series |
|---|---:|---:|---:|
| Semantic value exact | 119,663 | 4,095 | 29.2 |
| Logs-token fuzzy 0.5 | 737,315 | 2,559 | 288.1 |
| Semantic value adaptive 0.5 | 740,280 | 2,015 | 367.4 |
| Semantic value equal-length 0.5 | 730,211 | 4,218 | 173.1 |
| Current semantic type-aware 0.5 | 724,423 | 4,632 | 156.4 |

Allowing different token counts to match makes the adaptive semantic-value arm much denser, but barely changes its aggregate detector accuracy. Requiring equal lengths produces density close to the current semantic matcher and is the cleaner clustering control.

## Performance decomposition

The same 1,000-line mixed corpus was benchmarked in one invocation on an Apple M4 Max (`benchtime=500ms`, `count=5`, `benchmem`). Values are medians.

| Primitive/path | Time | Bytes | Allocations |
|---|---:|---:|---:|
| Logs tokenizer only | 199.3 ns/log | 272 B/log | 2/log |
| Semantic tokenizer only | 583.6 ns/log | 1,152 B/log | 1/log |
| Logs positional comparison | 12.5 ns/comparison | 0 B | 0 |
| Semantic `(type, value)` positional comparison | 58.5 ns/comparison | 0 B | 0 |
| Semantic type-aware clustering, pre-tokenized | 363.5 ns/log | 149 B/log | 1/log |
| Full Logs fuzzy extractor | 1.089 us/log | 1,275 B/log | 21/log |
| Full current semantic extractor | 1.655 us/log | 2,305 B/log | 19/log |

The semantic tokenizer itself is 2.9x slower and allocates 4.2x as many bytes as the Logs tokenizer. In the complete existing extractors, semantic is 1.52x slower and allocates 1.81x as many bytes.

On this corpus, the tokenizer delta accounts for approximately 68% of the complete CPU-time gap and 85% of the allocated-byte gap. Matching is measurably more expensive with rich tokens, but tens of nanoseconds per candidate; candidate count and signature-bucket fallback determine how much that grows in production.

The semantic-value ablation extractor was written for accuracy isolation, not optimized performance. Its full-path timing should not be treated as a production candidate measurement.

## Conclusion

The expensive part we would be giving up is primarily the **rich token representation**, not wildcard evolution or the type-aware compatibility checks. Those checks provide almost no aggregate F1 gain in the current 12-scenario set once semantic tokenization is retained.

This means simply porting the semantic clusterer onto the compact Logs tokens is unlikely to recover the missing accuracy: the Logs tokens have already discarded distinctions such as full paths, URI structure, date formats, IPs, emails, and key/value shapes.

The most promising reuse path is therefore:

1. Keep the Logs tokenizer and lightweight fuzzy matcher.
2. Identify which semantic token classes account for the 0.037 mean-F1 gap through recognizer ablations.
3. Add only the valuable, cheap structural promotions or matching guards to the shared Logs tokenizer/matcher.
4. Re-run on a broader scenario set and production-like source/cardinality workloads before removing the semantic parser.

Complete per-scenario results and benchmark medians are stored in `tokenizer-clustering-factorial-report.json`.
