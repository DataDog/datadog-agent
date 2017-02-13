## package `aggregator`

The Aggregator is the first thing a metric hits during its journey towards the intake.
The package is responsible to receive metrics, aggregate results, compute rates and
histograms, then it passes the ball to the Forwarder.

### Aggregator
For now the package provides only one Aggregator implementation, the `BufferedAggregator`,
named after its capabilities of storing in memory the samples it receives. The Aggregator
should be used as a singleton, the function `InitAggregator` takes care of this and should be
considered the right way to get an Aggregator instance at any time. An Aggregator has its own
loop that needs to be started with the `run` method, in the case of the `BufferedAggregator`
the buffer is flushed at defined intervals. An Aggregator receives metric samples using one 
or more channels and those samples are processed by different Samplers.

### Sampler
Metrics come this way as samples (e.g. in case of rates, the actual metric is computed over samples
in a given time) and Samplers take care of store and process them depending on where samples
come from. We currently use two different Samplers, one for samples coming from Dogstatsd, the other
one for samples coming from checks. In the latter case, we have one Sampler instance per check instance
(this is to support running the same check at different intervals).

### Sender
The Sender is used by calling code (namely: checks) that wants to send metric samples upstream. 
Sender exposes a high level interface mapping to different metric types supported upstream 
(Gauges, Counters, etc). To get an instance of the global default sender, call `GetDefaultSender`,
the function will take care of initialising everything, Aggregator included.

