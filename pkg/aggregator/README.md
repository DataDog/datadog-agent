## package `aggregator`

The Aggregator is the first thing a metric hits during its journey towards the
intake. This package is responsible for metrics reception and aggregation
before passing them to the forwarder. It computes rates and histograms and
passes them to the **Serializer**.

For now sources of metrics are DogStatsD and Python/Go checks. DogStatsD
directly send **MetricSample** to the Aggregator while checks use the sender to
do so.

**MetricSample** are the raw metric value that flow from our 2 sources to the
different metric types (Gauge, Count, ...).


         +===========+                       +===============+
         + DogStatsD +                       +    checks     +
         +===========+                       | Python and Go |
              ++                             +===============+
              ||                                    ++
              ||                                    vv
              ||                                .+------+.
              ||                                . Sender .
              ||                                '+---+--+'
              ||                                     |
              vv                                     v
           .+----------------------------------------+--+.
           +                 Aggregator                  +
           '+--+-------------------------------------+--+'
               |                                     |
               |                                     |
               v                                     v
        .+-----+-----+.                       .+-----+------+.
        + TimeSampler +                       + CheckSampler +
        '+-----+-----+'                       '+-----+------+'
               |                                     |
               |                                     |
               +         .+---------------+.         +
               '+------->+ ContextMetrics  +<-------+'
                         '+-------+-------+'
                                  |
                                  v
                         .+-------+-------+.
                         +     Metrics     +
                         | Gauge           |
                         | Count           |
                         | Histogram       |
                         | Rate            |
                         | Set             |
                         + ...             +
                         '+--------+------+'
                                  ||               +=================+
                                  ++==============>+  Serializer     |
                                                   +=================+

### Sender
The Sender is used by calling code (namely: checks) that wants to send metric
samples upstream. Sender exposes a high level interface mapping to different
metric types supported upstream (Gauges, Counters, etc). To get an instance of
the global default sender, call `GetDefaultSender`, the function will take care
of initialising everything, Aggregator included.

### Aggregator
For now the package provides only one Aggregator implementation, the
`BufferedAggregator`, named after its capabilities of storing in memory the
samples it receives. The Aggregator should be used as a singleton, the function
`InitAggregator` takes care of this and should be considered the right way to
get an Aggregator instance at any time. An Aggregator has its own loop that
needs to be started with the `run` method, in the case of the
`BufferedAggregator` the buffer is flushed at defined intervals. An Aggregator
receives metric samples using one or more channels and those samples are
processed by different samplers (`TimeSampler` or `CheckSampler`).

### Sampler
Metrics come this way as samples (e.g. in case of rates, the actual metric is
computed over samples in a given time) and samplers take care of store and
process them depending on where samples come from. We currently use two
different samplers, one for samples coming from Dogstatsd, the other one for
samples coming from checks. In the latter case, we have one sampler instance
per check instance (this is to support running the same check at different
intervals).

### Metric
We have different kind of metrics (Gauge, Count, ...). Those are responsible to
compute final `Serie` (set of points) to forwarde the the Datadog backend.
