# Log extractor comparison

All arms disable JSON numeric extraction and explicit connection-error extraction. Evaluations use the default detector/correlator pipeline with a Gaussian scoring sigma of 30 seconds.

| Arm | Branch | Mean F1 | Scenarios |
|---|---|---:|---:|
| Logs tokenizer, exact token hash | `eokye/tokenizer_use` | 0.1017195873 | 12 |

`io-contention` is omitted because no recording exists in the official scenario index or S3 bucket.
