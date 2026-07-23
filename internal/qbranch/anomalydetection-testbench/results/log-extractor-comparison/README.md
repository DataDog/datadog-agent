# Log extractor comparison

See [`TOKENIZER_CLUSTERING_FACTORIAL.md`](TOKENIZER_CLUSTERING_FACTORIAL.md) for the controlled performance and accuracy decomposition of Logs tokenization, semantic tokenization, fixed-representative matching, and type-aware matching.

See [`SEMANTIC_PATTERN_ATTRIBUTION.md`](SEMANTIC_PATTERN_ATTRIBUTION.md) for
the pattern-level attribution, detector case studies, and UUID causal
experiment.

All arms disable JSON numeric extraction and explicit connection-error extraction. Evaluations use the default detector/correlator pipeline with a Gaussian scoring sigma of 30 seconds.

| Arm | Branch | Mean F1 | Scenarios |
|---|---|---:|---:|
| Logs tokenizer, exact token hash | `eokye/tokenizer_use` | 0.1017195873 | 12 |
| Logs tokenizer, fuzzy 0.9 match | `eokye/tokenizer_fuzzy` | 0.1770880232 | 12 |

`io-contention` is omitted because no recording exists in the official scenario index or S3 bucket.
