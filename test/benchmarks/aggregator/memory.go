package main

import (
	"expvar"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/test/util"
	log "github.com/cihub/seelog"
)

func preAllocateMetrics(n int) map[string][]*metrics.MetricSample {

	metricMap := make(map[string][]*metrics.MetricSample)
	metricTemplate := "benchmark.metric." + util.RandomString(7)

	for mType := metrics.GaugeType; mType <= metrics.DistributionType; mType++ {
		metricName := metricTemplate + "_" + metrics.MetricType(mType).String()
		samples := make([]*metrics.MetricSample, n)

		for i, _ := range samples {
			value := float64(rand.Intn(1024))
			s := &metrics.MetricSample{
				Name:       metricName,
				Value:      value,
				Mtype:      mType,
				Tags:       []string{"a", "b:21", "c"},
				Host:       "localhost",
				SampleRate: 1,
				Timestamp:  util.TimeNowNano(),
			}
			samples[i] = s
		}
		metricType := metrics.MetricType(mType).String()
		metricMap[metricType] = samples
	}

	return metricMap
}

func preAllocateEvents(n int) []*metrics.Event {
	events := make([]*metrics.Event, n)

	for i, _ := range events {
		event := &metrics.Event{
			Title:          "Event title",
			Text:           "some text",
			Ts:             21,
			Priority:       metrics.EventPriorityNormal,
			Host:           "localhost",
			Tags:           []string{"a", "b:21", "c"},
			AlertType:      metrics.EventAlertTypeWarning,
			AggregationKey: "",
			SourceTypeName: "",
			EventType:      "",
		}
		events[i] = event
	}

	return events
}

func preAllocateServiceChecks(n int) []*metrics.ServiceCheck {
	scs := make([]*metrics.ServiceCheck, n)

	for i, _ := range scs {
		sc := &metrics.ServiceCheck{
			CheckName: "benchmark.sc." + util.RandomString(4),
			Status:    metrics.ServiceCheckOK,
			Host:      "localhost",
			Ts:        time.Now().Unix(),
			Tags:      []string{"a", "b:21", "c"},
			Message:   "foo",
		}
		scs[i] = sc
	}

	return scs
}

func benchmarkMemory(agg *aggregator.BufferedAggregator, sender aggregator.Sender, series, points []int, ips, dur int) {
	defer log.Flush()

	var wg sync.WaitGroup
	ticker := time.NewTicker(time.Second / time.Duration(ips))

	// Get raw sender
	rawSender, ok := sender.(aggregator.RawSender)
	if !ok {
		log.Error("[aggregator] sender not RawSender - cannot continue with benchmark")
		return
	}

	// Get memory stats from expvar:
	memstatsFunc := expvar.Get("memstats").(expvar.Func)
	memstats := memstatsFunc().(runtime.MemStats)

	quitGenerator := make(chan bool)

	for _, s := range series {
		for _, p := range points {
			// pre-allocate for operational memory usage benchmarking
			metrics := make([]map[string][]*metrics.MetricSample, s)
			for i := range metrics {
				metrics[i] = preAllocateMetrics(p)
			}
			scs := preAllocateServiceChecks(p)
			events := preAllocateEvents(p)
			sent := 0

			wg.Add(1)
			go func() {
				defer wg.Done()

				i := 0
				for _ = range ticker.C {
					i += 1
					i = i % p
					select {
					case <-quitGenerator:
						return
					default:
						// Submit Metrics
						for _, m := range metrics {
							for _, generated := range m {
								rawSender.SendRawMetricSample(generated[i])
								sent += 1
							}
						}

						// Submit ServiceCheck
						rawSender.SendRawServiceCheck(scs[i])
						sent += 1

						// Submit Event
						rawSender.Event(*events[i])
						sent += 1
					}
				}
			}()

			wg.Add(1)
			go func() {
				log.Infof("[aggregator] starting memory statter")
				initial := memstats.Alloc
				tickChan := time.NewTicker(time.Second).C
				defer wg.Done()

				secs := 0
				for _ = range tickChan {
					current := memstats.Alloc
					log.Infof("[aggregator] allocated: %v delta: %v mallocs: %v ", current, current-initial, memstats.Mallocs)
					if secs == dur {
						quitGenerator <- true
						return
					} else {
						secs += 1
					}
				}
			}()

			wg.Wait()
			log.Infof("[aggregator] benchmark concluded at a rate of %v pps (avg over %v secs)", sent/dur, dur)

		}
	}
}
