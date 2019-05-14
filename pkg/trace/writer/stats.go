package writer

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const pathStats = "/api/v0.2/stats"

// StatsWriter ingests stats buckets and flushes them to the API.
type StatsWriter struct {
	sender payloadSender
	exit   chan struct{}

	// InStats is the stream of stat buckets to send out.
	InStats <-chan []stats.Bucket

	// info contains various statistics about the writer, which are
	// occasionally sent as metrics to Datadog.
	info info.StatsWriterInfo

	// hostName specifies the resolved host name on which the agent is
	// running, to be sent as part of a stats payload.
	hostName string

	// env is environment this agent is configured with, to be sent as part
	// of the stats payload.
	env string

	conf writerconfig.StatsWriterConfig
}

// NewStatsWriter returns a new writer for stats.
func NewStatsWriter(conf *config.AgentConfig, InStats <-chan []stats.Bucket) *StatsWriter {
	cfg := conf.StatsWriterConfig
	endpoints := newEndpoints(conf, pathStats)
	sender := newMultiSender(endpoints, cfg.SenderConfig)
	log.Infof("Stats writer initializing with config: %+v", cfg)

	return &StatsWriter{
		sender:   sender,
		exit:     make(chan struct{}),
		InStats:  InStats,
		hostName: conf.Hostname,
		env:      conf.DefaultEnv,
		conf:     cfg,
	}
}

// Start starts the writer, awaiting stat buckets and flushing them.
func (w *StatsWriter) Start() {
	w.sender.Start()

	go func() {
		defer watchdog.LogOnPanic()
		w.Run()
	}()

	go func() {
		defer watchdog.LogOnPanic()
		w.monitor()
	}()
}

// Run runs the event loop of the writer's main goroutine. It reads stat buckets
// from InStats, builds stat payloads and sends them out using the base writer.
func (w *StatsWriter) Run() {
	defer close(w.exit)

	log.Debug("Starting stats writer")

	for {
		select {
		case stats := <-w.InStats:
			w.handleStats(stats)
		case <-w.exit:
			log.Info("Exiting stats writer")
			return
		}
	}
}

// Stop stops the writer
func (w *StatsWriter) Stop() {
	w.exit <- struct{}{}
	<-w.exit
	w.sender.Stop()
}

func (w *StatsWriter) handleStats(s []stats.Bucket) {
	payloads, nbStatBuckets, nbEntries := w.buildPayloads(s, w.conf.MaxEntriesPerPayload)
	if len(payloads) == 0 {
		return
	}

	defer timing.Since("datadog.trace_agent.stats_writer.encode_ms", time.Now())

	log.Debugf("Going to flush %v entries in %v stat buckets in %v payloads",
		nbEntries, nbStatBuckets, len(payloads),
	)

	if len(payloads) > 1 {
		atomic.AddInt64(&w.info.Splits, 1)
	}
	atomic.AddInt64(&w.info.StatsBuckets, int64(nbStatBuckets))

	headers := map[string]string{
		languageHeaderKey:  strings.Join(info.Languages(), "|"),
		"Content-Type":     "application/json",
		"Content-Encoding": "gzip",
	}

	for _, p := range payloads {
		// synchronously send the payloads one after the other
		data, err := stats.EncodePayload(p)
		if err != nil {
			log.Errorf("Encoding issue: %v", err)
			return
		}

		payload := newPayload(data, headers)
		w.sender.Send(payload)

		atomic.AddInt64(&w.info.Bytes, int64(len(data)))
	}
}

type timeWindow struct {
	start, duration int64
}

// buildPayloads returns a set of payload to send out, each paylods guaranteed
// to have the number of stats buckets under the given maximum.
func (w *StatsWriter) buildPayloads(s []stats.Bucket, maxEntriesPerPayloads int) ([]*stats.Payload, int, int) {
	if len(s) == 0 {
		return []*stats.Payload{}, 0, 0
	}

	// 1. Get an estimate of how many payloads we need, based on the total
	//    number of map entries (i.e.: sum of number of items in the stats
	//    bucket's count map).
	//    NOTE: we use the number of items in the count map as the
	//    reference, but in reality, what take place are the
	//    distributions. We are guaranteed the number of entries in the
	//    count map is > than the number of entries in the distributions
	//    maps, so the algorithm is correct, but indeed this means we could
	//    do better.
	nbEntries := 0
	for _, s := range s {
		nbEntries += len(s.Counts)
	}

	if maxEntriesPerPayloads <= 0 || nbEntries < maxEntriesPerPayloads {
		// nothing to do, break early
		return []*stats.Payload{{
			HostName: w.hostName,
			Env:      w.env,
			Stats:    s,
		}}, len(s), nbEntries
	}

	nbPayloads := nbEntries / maxEntriesPerPayloads
	if nbEntries%maxEntriesPerPayloads != 0 {
		nbPayloads++
	}

	// 2. Create a slice of nbPayloads maps, mapping a time window (stat +
	//    duration) to a stat bucket. We will build the payloads from these
	//    maps. This allows is to have one stat bucket per time window.
	pMaps := make([]map[timeWindow]stats.Bucket, nbPayloads)
	for i := 0; i < nbPayloads; i++ {
		pMaps[i] = make(map[timeWindow]stats.Bucket, nbPayloads)
	}

	// 3. Iterate over all entries of each stats. Add the entry to one of
	//    the payload container mappings, in a round robin fashion. In some
	//    edge cases, we can end up having the same entry in several
	//    inputted stat buckets. We must check that we never overwrite an
	//    entry in the new stats buckets but cleanly merge instead.
	i := 0
	for _, b := range s {
		tw := timeWindow{b.Start, b.Duration}

		for ekey, e := range b.Counts {
			pm := pMaps[i%nbPayloads]
			newsb, ok := pm[tw]
			if !ok {
				newsb = stats.NewBucket(tw.start, tw.duration)
			}
			pm[tw] = newsb

			if _, ok := newsb.Counts[ekey]; ok {
				newsb.Counts[ekey].Merge(e)
			} else {
				newsb.Counts[ekey] = e
			}

			if _, ok := b.Distributions[ekey]; ok {
				if _, ok := newsb.Distributions[ekey]; ok {
					newsb.Distributions[ekey].Merge(b.Distributions[ekey])
				} else {
					newsb.Distributions[ekey] = b.Distributions[ekey]
				}
			}
			if _, ok := b.ErrDistributions[ekey]; ok {
				if _, ok := newsb.ErrDistributions[ekey]; ok {
					newsb.ErrDistributions[ekey].Merge(b.ErrDistributions[ekey])
				} else {
					newsb.ErrDistributions[ekey] = b.ErrDistributions[ekey]
				}
			}
			i++
		}
	}

	// 4. Create the nbPayloads payloads from the maps.
	nbStats := 0
	nbEntries = 0
	payloads := make([]*stats.Payload, 0, nbPayloads)
	for _, pm := range pMaps {
		pstats := make([]stats.Bucket, 0, len(pm))
		for _, sb := range pm {
			pstats = append(pstats, sb)
			nbEntries += len(sb.Counts)
		}
		payloads = append(payloads, &stats.Payload{
			HostName: w.hostName,
			Env:      w.env,
			Stats:    pstats,
		})

		nbStats += len(pstats)
	}
	return payloads, nbStats, nbEntries
}

// monitor runs the event loop of the writer's monitoring
// goroutine. It:
// - reads events from the payload sender's monitor channel, logs
//   them, send out statsd metrics, and updates the writer info
// - periodically dumps the writer info
func (w *StatsWriter) monitor() {
	monC := w.sender.Monitor()

	infoTicker := time.NewTicker(w.conf.UpdateInfoPeriod)
	defer infoTicker.Stop()

	for {
		select {
		case e, ok := <-monC:
			if !ok {
				return
			}

			switch e.typ {
			case eventTypeSuccess:
				url := e.stats.host
				log.Debugf("Flushed stat payload; url: %s, time:%s, size:%d bytes", url, e.stats.sendTime,
					len(e.payload.bytes))
				tags := []string{"url:" + url}
				metrics.Gauge("datadog.trace_agent.stats_writer.flush_duration",
					e.stats.sendTime.Seconds(), tags, 1)
				atomic.AddInt64(&w.info.Payloads, 1)
			case eventTypeFailure:
				url := e.stats.host
				log.Errorf("Failed to flush stat payload; url:%s, time:%s, size:%d bytes, error: %s",
					url, e.stats.sendTime, len(e.payload.bytes), e.err)
				atomic.AddInt64(&w.info.Errors, 1)
			case eventTypeRetry:
				log.Errorf("Retrying flush stat payload, retryNum: %d, delay:%s, error: %s",
					e.retryNum, e.retryDelay, e.err)
				atomic.AddInt64(&w.info.Retries, 1)
			default:
				log.Debugf("Unable to handle event with type %T", e)
			}

		case <-infoTicker.C:
			var swInfo info.StatsWriterInfo

			// Load counters and reset them for the next flush
			swInfo.Payloads = atomic.SwapInt64(&w.info.Payloads, 0)
			swInfo.StatsBuckets = atomic.SwapInt64(&w.info.StatsBuckets, 0)
			swInfo.Bytes = atomic.SwapInt64(&w.info.Bytes, 0)
			swInfo.Retries = atomic.SwapInt64(&w.info.Retries, 0)
			swInfo.Splits = atomic.SwapInt64(&w.info.Splits, 0)
			swInfo.Errors = atomic.SwapInt64(&w.info.Errors, 0)

			// TODO(gbbr): Scope these stats per endpoint (see (config.AgentConfig).AdditionalEndpoints))
			metrics.Count("datadog.trace_agent.stats_writer.payloads", int64(swInfo.Payloads), nil, 1)
			metrics.Count("datadog.trace_agent.stats_writer.stats_buckets", int64(swInfo.StatsBuckets), nil, 1)
			metrics.Count("datadog.trace_agent.stats_writer.bytes", int64(swInfo.Bytes), nil, 1)
			metrics.Count("datadog.trace_agent.stats_writer.retries", int64(swInfo.Retries), nil, 1)
			metrics.Count("datadog.trace_agent.stats_writer.splits", int64(swInfo.Splits), nil, 1)
			metrics.Count("datadog.trace_agent.stats_writer.errors", int64(swInfo.Errors), nil, 1)

			info.UpdateStatsWriterInfo(swInfo)
		}
	}
}
