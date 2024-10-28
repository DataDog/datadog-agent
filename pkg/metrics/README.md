# Metrics

The agent offers different type of metric. Each metrics offers 2 methods `addSample` and `flush`.

- `addSample`: add a new sample to the metrics.
- `flush`: aggregate all samples received since the last flush and return a
  `Series` to be forwarded to the Datadog backend.

### gauge

Gauge tracks the value of a metric. The last value received is the one returned
by the `flush` method.

### counter

Counter tracks how many times something happened per second. Counters are only
used by DogStatsD and are very similar to Count: the main diffence is that they
are sent as Rate.

### count

Count is used to count the number of events that occur between 2 flushes. Each
sample's value is added to the value that's flushed.

### histogram

Histogram tracks the distribution of samples added over one flush period.

### historate

Historate tracks the distribution of samples added over one flush period for
"rate" like metrics. Warning this doesn't use the harmonic mean, beware of what
it means when using it.

### monotonic_count

MonotonicCount tracks a raw counter, based on increasing counter values.
Samples that have a lower value than the previous sample are ignored (since it
usually means that the underlying raw counter has been reset).

Example:

Submitting samples `2`, `3`, `6`, `7` returns `5` (i.e. `7`-`2`) on flush, then submitting
samples `10`, `11` on the same MonotonicCount returns `4` (i.e. `11`-`7`) on the second flush.

### percentile

Percentile tracks the distribution of samples added over one flush period.
Designed to be globally accurate for percentiles.

Percentile is not usable yet; it is still undergoing development and testing.

### rate

Rate tracks the rate of a metric over 2 successive flushes (ie: no metrics will
be returned on the first flush).

### set

Set tracks the number of unique elements in a set. This is only used by
DogStatsD; you cannot create sets from an Agent check.

# service_check

Service checks track the status of any service: `OK`, `WARNING`, `CRITICAL`, or
`UNKNOWN`. The Agent does not aggregate service checks, it sends every check
straight to Datadog's backend.

# event

Events represent discrete moments in time, e.g. thrown exceptions, code
deploys, etc. The Agent does not aggregate events, it sends every event straight
to your Datadog event stream.
