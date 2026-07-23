# Semantic Pattern Attribution

## Question

Which log shapes explain the semantic tokenizer's accuracy advantage, and can
small additions to the Logs tokenizer recover that advantage without adopting
the heavier semantic parser?

## Method

The investigation used two controls:

1. Tokenization-only attribution ran semantic `(type, raw value)` tokens and
   compact Logs tokens through the same ordered, fixed-representative,
   positional matcher at threshold 0.5. It recorded the many-to-many mapping
   between the resulting clusters. Observer feedback logs were identified and
   excluded from the primary mapping rates.
2. Detector case studies were predeclared from both sides: three large semantic
   wins and three large Logs-tokenizer wins. This prevents selecting only
   examples that support a tokenizer change.

Seven scenarios containing 474,362 logs were analyzed. A cluster is considered
emitted after its fifth matching log, matching the experiment configuration.

## Cluster mapping

`Semantic fragmentation` is the share of logs in a semantic cluster that the
Logs tokenizer splits away from that cluster's largest Logs-tokenizer match.
`Logs overmerge` is the reverse: the share of a Logs cluster outside its largest
semantic-cluster match.

| Scenario | Logs | Semantic clusters | Logs clusters | Semantic fragmentation | Logs overmerge |
|---|---:|---:|---:|---:|---:|
| Block building | 28,356 | 159 | 182 | 29.6% | 10.3% |
| Cassandra repair | 87,343 | 206 | 298 | 80.6% | 30.5% |
| Kafka partition | 57,646 | 178 | 204 | 2.9% | 6.2% |
| Lock contention | 29,347 | 221 | 230 | 3.0% | 5.5% |
| Memcached | 178,819 | 182 | 270 | 15.9% | 4.1% |
| Pool saturation | 62,611 | 139 | 143 | 8.8% | 2.4% |
| Redis cascade | 30,240 | 205 | 345 | 26.0% | 17.7% |

The strongest associations with fragmentation were structured JSON, long
hexadecimal and integer values, UUIDs, key/value logs, dates, severities, and
HTTP fields. These are associations, not individual causal effects, because
several constructs commonly occur in the same log.

## Detector-level findings

| Case | Semantic F1 | Logs fuzzy F1 | What caused the difference |
|---|---:|---:|---|
| Cassandra + ScanMW | 0.9868 | ~0 | UUIDs were split into different `C`/`D` token sequences according to their random hexadecimal contents. The anomaly-producing integration-api event pattern therefore fragmented across many series. |
| Pool + BOCPD | 0.8326 | 0.3729 | The semantic tokens formed a broader cluster spanning login-attempt and new-connection messages. Both found the disruption, but Logs fuzzy also produced two baseline false positives. This is an aggregation/granularity effect, not evidence for one missing value recognizer. |
| Block + Holt | 0.9451 | 0.1856 | Semantic token compression grouped access logs across changing methods, paths, IPs, and trace values. Logs fuzzy retained more series and produced eight baseline false positives. |
| Kafka + ScanMW | 0 | 0.9868 | The Logs cluster accidentally combined analytics-query logs with unrelated DogStatsD packet errors. That overmerge strengthened the onset signal; it is not a behavior to deliberately preserve. |
| Memcached + Holt | ~0 | 0.9138 | A broad Logs nginx cluster combined normal traffic with disruption-time 503 access logs. The aggregation made the change visible, but was less semantically pure. |
| Block + ScanWelch | 0 | 0.5383 | Broad Logs access-log aggregation exposed the disruption. As above, this is evidence that series granularity matters, not that compact tokenization is intrinsically more accurate. |

The main result is narrower than “semantic parsing is better”: high-entropy
structured values can fragment the compact token stream, while broad merges can
either suppress false positives or accidentally create a useful signal. The
detector and cluster granularity interact.

## Causal tokenizer experiment: UUID

The Logs tokenizer was extended in the experiment branch with one structural
token, `UUID`. A standard 36-character UUID is now represented by one token
instead of a value-dependent sequence of letter, digit, and dash tokens.

On Cassandra + ScanMW:

| Metric | Before UUID | With UUID |
|---|---:|---:|
| F1 | ~0 | **0.9868** |
| Precision | 1.0000 | 1.0000 |
| Recall | ~0 | 0.9739 |
| Logs clusters | 298 | 254 |
| Semantic fragmentation | 80.6% | 18.9% |
| Semantic-only emitted logs | 455 | 286 |

This exactly recovered the semantic-token result for the selected causal case.
The five non-Cassandra detector controls were unchanged: pool BOCPD 0.3729,
block Holt 0.1856, Kafka ScanMW 0.9868, memcached Holt 0.9138, and block
ScanWelch 0.5383.

## Performance

The UUID recognizer is integrated into the tokenizer's existing byte pass and
adds no allocation.

| Benchmark | Before | With UUID | Change |
|---|---:|---:|---:|
| Logs tokenization | 302.6 ns/log, 272 B, 2 allocs | 336.0 ns/log, 272 B, 2 allocs | +11.0% time |
| Full fuzzy extractor | 1,648 ns/log, 1,275 B, 21 allocs | 1,697 ns/log, 1,275 B, 21 allocs | +3.0% time |

For context in the same run, the full semantic type-aware extractor took
2,548 ns/log and 2,305 B/log. The UUID-enhanced fuzzy path therefore remained
about 1.5x faster and used about 45% fewer allocated bytes.

## Recommendation

Do not port the semantic parser or its evolving clusterer wholesale yet.
Instead, keep the Logs tokenizer and add cheap structural tokens for
high-entropy values where a held-out detector case proves a causal benefit.

UUID is a strong first candidate: the accuracy recovery is causal, large, and
localized, while full-extractor overhead is about 3%. The next candidates should
be tested one at a time in this order:

1. long hexadecimal identifiers;
2. IP/authority values;
3. paths or URIs.

Each addition should pass the same gates: reduced fragmentation in the target
patterns, improved detector F1 on a predeclared case, no regression in
predeclared controls or a broader scenario matrix, and acceptable
production-like CPU/cardinality impact. JSON parsing as a whole is not
recommended based on this evidence; it is too broad a feature association to
justify the cost.

## Limitations

- Pattern attribution covered seven scenarios; the existing aggregate accuracy
  table covers twelve.
- The UUID change was replayed against one causal win and five controls, not the
  full detector-by-scenario matrix.
- Some compact-tokenizer wins came from accidental overmerge, so F1 alone
  cannot be used as the clustering-quality objective.
- The microbenchmark corpus is controlled and allocation-sensitive, but a
  production-rate benchmark is still required before shipping the recognizer.
