// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	log "github.com/cihub/seelog"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

type senderFunc func(string, float64, string, []string)

func generateMetrics(numberOfSeries int, pointPerSeries int, senderMetric senderFunc) float64 {
	start := time.Now()
	for s := 0; s < numberOfSeries; s++ {
		serieName := "benchmark.metric." + strconv.Itoa(s)
		for p := 0; p < pointPerSeries; p++ {
			senderMetric(serieName, float64(rand.Intn(1024)), "localhost", []string{"a", "b:21", "c"})
		}
	}
	waitForAggregatorEmptyQueue()
	return float64(time.Since(start)) / float64(time.Millisecond)
}

func generateEvent(numberOfEvent int, sender sender.Sender) float64 {
	start := time.Now()
	for i := 0; i < numberOfEvent; i++ {
		sender.Event(event.Event{
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
		})
	}
	return float64(time.Since(start)) / float64(time.Millisecond)
}

func generateServiceCheck(numberOfSC int, sender sender.Sender) float64 {
	start := time.Now()
	for i := 0; i < numberOfSC; i++ {
		sender.ServiceCheck("benchmark.ServiceCheck."+strconv.Itoa(i), servicecheck.ServiceCheckOK, "localhost", []string{"a", "b:21", "c"}, "some message")
	}
	return float64(time.Since(start)) / float64(time.Millisecond)
}

func benchmarkMetrics(numberOfSeries []int, nbPoints []int, sender sender.Sender, info *aggregatorStats, branchName string) []datadog.Metric {
	metrics := map[string]senderFunc{"Gauge": sender.Gauge,
		"Rate":           sender.Rate,
		"Count":          sender.Count,
		"MonotonicCount": sender.MonotonicCount,
		"Histogram":      sender.Histogram,
		"Historate":      sender.Historate,
	}
	// this is here to keep the same order between the header and metric
	metricTypes := []string{"Gauge", "Rate", "Count", "MonotonicCount", "Histogram", "Historate"}

	t := time.Now().Unix()
	results := []datadog.Metric{}
	for _, nbSerie := range numberOfSeries {
		log.Infof("-- Series of %d points ---\n", nbSerie)
		for _, name := range metricTypes {
			for _, nbPoint := range nbPoints {
				tags := []string{
					fmt.Sprintf("branch:%s", branchName),
					fmt.Sprintf("nb_point:%d", nbPoint),
					fmt.Sprintf("nb_serie:%d", nbSerie),
					fmt.Sprintf("type:%s", name),
				}

				genTime := generateMetrics(nbSerie, nbPoint, metrics[name])
				start := time.Now()
				sender.Commit()
				commitTime := float64(time.Since(start)) / float64(time.Millisecond)

				info = report(info, "ChecksMetricSampleFlushTime")
				flushTime := float64(info.Flush["ChecksMetricSampleFlushTime"].LastFlush) / float64(time.Millisecond)

				genRes := createMetric(genTime, tags, "benchmark.aggregator.gen", t)
				commitRes := createMetric(commitTime, tags, "benchmark.aggregator.commit", t)
				flushRes := createMetric(flushTime, tags, "benchmark.aggregator.flush", t)

				log.Infof("[%d %s] [%d point] sent %f | commit time: %f | flush time: %f", nbSerie, name, nbPoint, genTime, commitTime, flushTime)

				results = append(results, genRes)
				results = append(results, commitRes)
				results = append(results, flushRes)
			}
		}
	}

	log.Infof("-- Events ---")
	for _, nbSerie := range numberOfSeries {
		tags := []string{fmt.Sprintf("nb_serie:%d", nbSerie), fmt.Sprintf("branch:%s", branchName), "type:event"}

		genTime := generateEvent(nbSerie, sender)
		start := time.Now()
		sender.Commit()
		commitTime := float64(time.Since(start)) / float64(time.Millisecond)

		info := report(info, "EventFlushTime")
		flushTime := float64(info.Flush["EventFlushTime"].LastFlush) / float64(time.Millisecond)

		genRes := createMetric(genTime, tags, "benchmark.aggregator.gen", t)
		commitRes := createMetric(commitTime, tags, "benchmark.aggregator.commit", t)
		flushRes := createMetric(flushTime, tags, "benchmark.aggregator.flush", t)

		results = append(results, genRes)
		results = append(results, commitRes)
		results = append(results, flushRes)
		log.Infof("[%d Event] sent %f | commit time: %f | flush time: %f", nbSerie, genTime, commitTime, flushTime)
	}

	log.Infof("-- ServiceChecks ---")
	for _, nbSerie := range numberOfSeries {
		tags := []string{fmt.Sprintf("nb_serie:%d", nbSerie), fmt.Sprintf("branch:%s", branchName), "type:service_check"}

		genTime := generateServiceCheck(nbSerie, sender)
		start := time.Now()
		sender.Commit()
		commitTime := float64(time.Since(start)) / float64(time.Millisecond)

		info := report(info, "ServiceCheckFlushTime")
		flushTime := float64(info.Flush["ServiceCheckFlushTime"].LastFlush) / float64(time.Millisecond)

		genRes := createMetric(genTime, tags, "benchmark.aggregator.gen", t)
		commitRes := createMetric(commitTime, tags, "benchmark.aggregator.commit", t)
		flushRes := createMetric(flushTime, tags, "benchmark.aggregator.flush", t)

		results = append(results, genRes)
		results = append(results, commitRes)
		results = append(results, flushRes)
		log.Infof("[%d ServiceCheck] sent %f | commit time: %f | flush time: %f", nbSerie, genTime, commitTime, flushTime)
	}

	return results
}
