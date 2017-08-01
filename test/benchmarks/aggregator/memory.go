package main

import (
	"bytes"
	"expvar"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/test/util"
	log "github.com/cihub/seelog"
)

var memGnuplotHeader = `
set term x11 %d
set title sprintf("%d points, %d series (%d pps)", rate_%d_%d)
set xlabel "Time (s)"
set ylabel "Bytes"
set y2label "Objects"
set y2tics
set key outside box opaque
plot %s using 1:2 w lines title "allocated" axes x1y1, %s using 1:3 w lines title "delta" axes x1y1, %s using 1:5 w lines title "live objects" axes x1y2
`

var memRateGnuplotHeader = `
set term x11 %d
set title "Rate vs Memory"
set xlabel "Rate (pps)"
set ylabel "Bytes"
set y2label "Objects"
set y2tics
set key outside box opaque
plot $data_rates using 1:2 w impulses title "allocated" axes x1y1, $data_rates using 1:3 w impulses title "objects" axes x1y2
`

func preAllocateMetrics(n int) map[string][]*metrics.MetricSample {

	metricMap := make(map[string][]*metrics.MetricSample)
	metricTemplate := "benchmark.metric." + util.RandomString(7)

	for mType := metrics.GaugeType; mType <= metrics.DistributionType; mType++ {
		// Not supported for now
		if mType == metrics.DistributionType {
			continue
		}
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

func benchmarkMemory(agg *aggregator.BufferedAggregator, sender aggregator.Sender, series, points []int, ips, dur int) string {
	var dataBuf, plotBuf bytes.Buffer
	var wg sync.WaitGroup

	ticker := time.NewTicker(time.Second / time.Duration(ips))

	rates := make(map[int]runtime.MemStats)
	// Get raw sender
	rawSender, ok := sender.(aggregator.RawSender)
	if !ok {
		log.Error("[aggregator] sender not RawSender - cannot continue with benchmark")
		return ""
	}

	// Get memory stats from expvar:
	memstatsFunc := expvar.Get("memstats").(expvar.Func)

	quitGenerator := make(chan bool)

	iteration := 0
	for _, s := range series {
		for _, p := range points {
			dataBuf.Reset()
			dataBuf.WriteString(fmt.Sprintf("$data_%d_%d <<EOD\n", s, p))

			// pre-allocate for operational memory usage benchmarking
			metrics := make([]map[string][]*metrics.MetricSample, s)
			for i := range metrics {
				metrics[i] = preAllocateMetrics(p)
			}
			scs := preAllocateServiceChecks(p)
			events := preAllocateEvents(p)
			sent := 0

			// get memory stats
			initial := memstatsFunc().(runtime.MemStats)

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
				log.Infof("[aggregator] starting memory statter (%d points per series, %d series)", p, s)
				tickChan := time.NewTicker(time.Second).C
				defer wg.Done()

				// get memory stats
				prev := initial

				secs := 0
				for _ = range tickChan {
					current := memstatsFunc().(runtime.MemStats)
					delta := int64(current.Alloc - prev.Alloc)
					mallocDelta := int64(current.Mallocs - prev.Mallocs)
					live := current.Mallocs - current.Frees
					dataBuf.WriteString(fmt.Sprintf("%5d %10d %10d %10d %10d\n", secs, current.Alloc, delta, mallocDelta, live))
					log.Infof("[aggregator] allocated: %10d\tdelta: %10d mallocs: %10d live objects:%10d", current.Alloc, delta, mallocDelta, live)
					prev = current
					if secs == dur {
						log.Infof("[aggregator] total memory delta %d bytes", int64(current.Alloc-initial.Alloc))
						log.Infof("[aggregator] benchmark concluded at a rate of %v pps (avg over %v secs)", sent/dur, dur)
						rates[sent/dur] = current
						quitGenerator <- true
						dataBuf.WriteString("EOD\n\n")
						return
					} else {
						secs += 1
					}
				}
			}()

			wg.Wait()
			dataBuf.WriteString(fmt.Sprintf("rate_%d_%d = %v", s, p, sent/dur))

			plotBuf.WriteString(dataBuf.String())
			plotBuf.WriteString("\n")
			dataset := fmt.Sprintf("$data_%d_%d", s, p)
			plotBuf.WriteString(fmt.Sprintf(memGnuplotHeader, iteration, p, s, sent/dur, p, s, dataset, dataset, dataset))
			plotBuf.WriteString("\n")

			iteration += 1
		}
	}

	plotBuf.WriteString("\n$data_rates <<EOD\n")
	for rate, stats := range rates {
		plotBuf.WriteString(fmt.Sprintf("%10d %10d %10d\n", rate, stats.Alloc, stats.Mallocs-stats.Frees))
	}
	plotBuf.WriteString("EOD\n\n")
	plotBuf.WriteString(fmt.Sprintf(memRateGnuplotHeader, iteration))

	return plotBuf.String()
}
