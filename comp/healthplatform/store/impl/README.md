# Caller latency contract

Healthplatform runs on critical paths such as Autodiscovery while its caller
holds a lock. Every method of `store.Component` must return with p99 latency
below **100 ms**, including when identity resolution or persistence is wedged.
The budget includes lock and queue waits. These tests measure caller return
time, not eventual processing or persistence time.

`store_stress_test.go` runs with the ordinary `test` build tag, including short
test runs. Its inclusion in `impl_test` in `BUILD.bazel` puts it in the existing
GitLab Bazel test and race/coverage jobs on the supported platforms. No opt-in
stress tag, benchmark invocation, or separate CI job is required.

The suite checks:

- Deployment and cluster resolvers that never return until test cleanup, even
  when given a canceled context.
- Disk writes that stall, fail immediately, or stall and then fail. Faults wrap
  the real disk persistence implementation. A priming report must reach the
  fault before measurements begin; the persistence timer has up to 35 seconds
  to do so.
- Bursts from 64 producers submitting 16,384 report/resolve calls, with p99
  calculated separately for each method. A five-second watchdog fails the test
  if any caller remains stuck; incomplete calls cannot disappear from p99.
- A health_platform counter whose name contains `drop` must increase on
  saturation. Once the consumer is wedged and the queue is full, each subsequent
  report, resolve, and resolve-all call must increment the drop count once.
  The saturation burst must exceed the event queue's capacity.
- Every exported store method, including queries, observer registration, and
  identity helpers, under the same faults for node and cluster agents. Adding
  an interface method requires adding its latency case.

Run through the normal test entry point:

```shell
dda inv test --targets=./comp/healthplatform/store/impl --race
```

These are regression gates for the asynchronous store implementation. They
intentionally fail against the synchronous implementation: stack/merge this
change after the asynchronous report/resolve and bounded-queue implementation.
Moving only report/resolve to a worker is insufficient if exported identity
helpers still wait on selfident, or queries wait on a lock held during I/O.
Faults are explicitly released before callers are joined and the store stops.
