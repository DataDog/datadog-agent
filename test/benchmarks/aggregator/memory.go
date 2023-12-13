// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"expvar"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	log "github.com/cihub/seelog"

	"gopkg.in/zorkian/go-datadog-api.v2"
)

func preAllocateMetrics(n int) map[string][]*metrics.MetricSample {

	metricMap := make(map[string][]*metrics.MetricSample)
	metricTemplate := "benchmark.metric." + RandomString(7)

	for mType := metrics.GaugeType; mType <= metrics.DistributionType; mType++ {
		// Not supported for now
		if mType == metrics.DistributionType {
			continue
		}
		metricName := metricTemplate + "_" + metrics.MetricType(mType).String()
		samples := make([]*metrics.MetricSample, n)

		for i := range samples {
			value := float64(rand.Intn(1024))
			s := &metrics.MetricSample{
				Name:       metricName,
				Value:      value,
				Mtype:      mType,
				Tags:       []string{"a", "b:21", "c"},
				Host:       "localhost",
				SampleRate: 1,
			}
			samples[i] = s
		}
		metricType := metrics.MetricType(mType).String()
		metricMap[metricType] = samples
	}

	return metricMap
}

func preAllocateEvents(n int) []*event.Event {
	events := make([]*event.Event, n)

	for i := range events {
		event := &event.Event{
			Title:          "Event title",
			Text:           "some text",
			Ts:             21,
			Priority:       event.EventPriorityNormal,
			Host:           "localhost",
			Tags:           []string{"a", "b:21", "c"},
			AlertType:      event.EventAlertTypeWarning,
			AggregationKey: "",
			SourceTypeName: "",
			EventType:      "",
		}
		events[i] = event
	}

	return events
}

func preAllocateServiceChecks(n int) []*servicecheck.ServiceCheck {
	scs := make([]*servicecheck.ServiceCheck, n)

	for i := range scs {
		sc := &servicecheck.ServiceCheck{
			CheckName: "benchmark.sc." + RandomString(4),
			Status:    servicecheck.ServiceCheckOK,
			Host:      "localhost",
			Ts:        time.Now().Unix(),
			Tags:      []string{"a", "b:21", "c"},
			Message:   "foo",
		}
		scs[i] = sc
	}

	return scs
}

func benchmarkMemory(agg *aggregator.BufferedAggregator, sender sender.Sender, series, points []int,
	ips, dur int, branchName string) []datadog.Metric {

	results := []datadog.Metric{}
	var wg sync.WaitGroup

	ticker := time.NewTicker(time.Second / time.Duration(ips))

	// Get raw sender
	rawSender, ok := sender.(aggregator.RawSender)
	if !ok {
		log.Error("[aggregator] sender not RawSender - cannot continue with benchmark")
		return results
	}

	// Get memory stats from expvar:
	memstatsFunc := expvar.Get("memstats").(expvar.Func)

	quitGenerator := make(chan bool)

	for _, s := range series {
		for _, p := range points {
			tags := []string{
				fmt.Sprintf("branch:%s", branchName),
				fmt.Sprintf("points:%d", p),
				fmt.Sprintf("series:%d", s),
			}

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
				for range ticker.C {
					i++
					i = i % p
					select {
					case <-quitGenerator:
						return
					default:
						// Submit Metrics
						for _, m := range metrics {
							for _, generated := range m {
								rawSender.SendRawMetricSample(generated[i])
								sent++
							}
						}

						// Submit ServiceCheck
						rawSender.SendRawServiceCheck(scs[i])
						sent++

						// Submit Event
						rawSender.Event(*events[i])
						sent++
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
				for range tickChan {
					// compute metrics
					current := memstatsFunc().(runtime.MemStats)
					delta := float64(current.Alloc) - float64(prev.Alloc)
					mallocDelta := float64(current.Mallocs) - float64(prev.Mallocs)
					live := float64(current.Mallocs) - float64(current.Frees)

					t := time.Now().Unix()
					mAlloc := createMetric(float64(current.Alloc), tags, "benchmark.aggregator.mem.alloc", t)
					mDelta := createMetric(delta, tags, "benchmark.aggregator.mem.delta", t)
					mDeltaMalloc := createMetric(mallocDelta, tags, "benchmark.aggregator.mem.delta.malloc", t)
					mLive := createMetric(live, tags, "benchmark.aggregator.mem.live", t)

					// Append to result slice
					results = append(results, mAlloc)
					results = append(results, mDelta)
					results = append(results, mDeltaMalloc)
					results = append(results, mLive)

					log.Infof("[aggregator] allocated: %10d\tdelta: %11.f mallocs: %11.f live objects:%11.f", current.Alloc, delta, mallocDelta, live)
					prev = current
					if secs == dur {
						t := time.Now().Unix()
						delta = float64(current.Alloc) - float64(initial.Alloc)
						log.Infof("[aggregator] total memory delta %11.f bytes", delta)
						log.Infof("[aggregator] benchmark concluded at a rate of %v pps (avg over %v secs)", sent/dur, dur)
						results = append(results, createMetric(delta, tags, "benchmark.aggregator.mem.total_delta", t))
						results = append(results, createMetric(float64(sent/dur), tags, "benchmark.aggregator.mem.rate", t))
						quitGenerator <- true
						return
					}
					secs++
				}
			}()

			wg.Wait()
		}
	}

	return results
}
