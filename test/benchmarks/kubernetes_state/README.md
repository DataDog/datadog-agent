This tool is used to benchmark the KSMv2 core check implementation.

The first thing to do is to build the test data.
Indeed, test data are too big to be committed in git.
Here is how to build them:

```sh
test/benchmarks/kubernetes_state/testdata/generate.sh
```

As long as `generate.sh` isn’t called again, the benchmark will always run with the same dataset.
This allows a fair comparison between several runs of the benchmark.

The benchmark can be launched with the following `invoke` target:

```sh
time inv bench.kubernetes-state
```

The benchmark only returns the time it took to run the main function of the KSMv2 check on the static test data built at the previous state:

```
KSMCheck.Run() returned <nil> in 4m7.940078955s
```

The time is an elapsed time.
The benchmark also produces a CPU pprof profile that can be viewed with:

```sh
go tool pprof -web cpuprofile.pprof
```
