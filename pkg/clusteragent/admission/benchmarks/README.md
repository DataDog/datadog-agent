# Admission benchmarks

The matrix covers 1, 10, and 100 containers with either 5 environment variables
or 40 variables and a 16 KiB unknown payload. It measures no-op, annotation,
resource, toleration, normalization, configuration, SSI, and Fargate sidecar
requests. Configuration, SSI, and sidecar cases invoke their real webhooks;
the other cases measure the common runner with representative primitive intent.
The resource case does not benchmark recommendation evaluation or external reads.

Run on one host with:

```sh
dda inv test --targets=./pkg/clusteragent/admission/benchmarks --bazel-args='--test_output=all --test_arg=-test.run=^$ --test_arg=-test.bench=BenchmarkAdmission --test_arg=-test.benchmem --test_arg=-test.benchtime=100ms --test_arg=-test.count=3'
```

To compare against the source baseline
`83b46e21baedd3c25542aea28a706114f18a8b36`, use an isolated checkout. Copy
`benchmark_test.go` unchanged and copy `testdata/main_adapter.go.txt` as
`primitive_test.go` into the same benchmark package there, then generate its
BUILD file with `bazel run //:gazelle -- ./pkg/clusteragent/admission/benchmarks`.
Leave production baseline files unchanged. Run the same command on that host.

The benchmark adds only synthetic fixtures. Dry-run sidecar requests suppress
ConfigMap writes, and SSI selection uses an explicit Java annotation. These
measurements do not establish API-server persistence, staging p99, or timeout
behavior. Baseline accidental removals and typed-serialization artifacts are
excluded from functional equivalence requirements.

SSI performance is deferred for the functional MVP. Early diagnostic medians
are retained in
[`admission-patching-benchmarks.csv`](../../../../doc/architecture/admission-patching-benchmarks.csv).
Those runs preceded the final snapshot-cache and read-after-write fixes; they
are not performance acceptance results for the final implementation. The early
baseline also predates the reporting of patch bytes and operation counts, so
those CSV cells are empty. Rerun this identical harness for a controlled final
comparison when evaluating performance gates.
