package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

var gnuplotHeader = `# set terminal png truecolor
# set autoscale
# set style data linespoints
# set xlabel "Number of points per serie"
# set ylabel "Time in Ms"
# set grid
# set key autotitle columnhead
#
# plot for [i=2:%d] "%s" u 1:i

`

type senderFunc func(string, float64, string, []string)

func generateMetrics(numberOfSeries int, pointPerSeries int, senderMetric senderFunc) time.Duration {
	start := time.Now()
	for s := 0; s < numberOfSeries; s++ {
		serieName := "benchmark.metric." + strconv.Itoa(s)
		for p := 0; p < pointPerSeries; p++ {
			senderMetric(serieName, float64(rand.Intn(1024)), "localhost", []string{"a", "b:21", "c"})
		}
	}
	return time.Since(start)
}

func generateEvent(numberOfEvent int, sender aggregator.Sender) time.Duration {
	start := time.Now()
	for i := 0; i < numberOfEvent; i++ {
		sender.Event(aggregator.Event{
			Title:          "Event title",
			Text:           "some text",
			Ts:             21,
			Priority:       aggregator.EventPriorityNormal,
			Host:           "localhost",
			Tags:           []string{"a", "b:21", "c"},
			AlertType:      aggregator.EventAlertTypeWarning,
			AggregationKey: "",
			SourceTypeName: "",
			EventType:      "",
		})
	}
	return time.Since(start)
}

func generateServiceCheck(numberOfSC int, sender aggregator.Sender) time.Duration {
	start := time.Now()
	for i := 0; i < numberOfSC; i++ {
		sender.ServiceCheck("benchmark.ServiceCheck."+strconv.Itoa(i), aggregator.ServiceCheckOK, "localhost", []string{"a", "b:21", "c"}, "some message")
	}
	return time.Since(start)
}

func benchmarkMetrics(agg *aggregator.BufferedAggregator, numberOfSeries []int, nbPoints []int, sender aggregator.Sender, flush chan time.Time, info *aggregatorStats) string {
	metrics := map[string]senderFunc{"Gauge": sender.Gauge,
		"Rate":           sender.Rate,
		"Count":          sender.Count,
		"MonotonicCount": sender.MonotonicCount,
		"Histogram":      sender.Histogram,
		"Historate":      sender.Historate,
	}
	// this is here to keep the same order between the header and metric
	metricTypes := []string{"Gauge", "Rate", "Count", "MonotonicCount", "Histogram", "Historate"}

	// add plot header
	plotRes := fmt.Sprintf(gnuplotHeader, len(metricTypes)*len(numberOfSeries)+2, *plotFile)
	plotRes += "nbPoint"
	for _, name := range metricTypes {
		for _, s := range numberOfSeries {
			plotRes += fmt.Sprintf(" %d-serie-%s", s, name)
		}
	}
	plotRes += " Event ServiceCheck"

	for _, nbPoint := range nbPoints {
		plotRes += fmt.Sprintf("\n%d", nbPoint)
		fmt.Printf("-- Series of %d points ---\n", nbPoint)
		for _, name := range metricTypes {
			for _, nbSerie := range numberOfSeries {
				genTime := generateMetrics(nbSerie, nbPoint, metrics[name])
				start := time.Now()
				sender.Commit()
				commitTime := time.Since(start)
				info = report(agg, flush, info, "ChecksMetricSampleFlushTime")

				fmt.Printf("sent %d %s series of %d poins in %f and commited in %f flush in %f serialize in %f\n",
					nbSerie, name, nbPoint,
					float64(genTime)/float64(time.Millisecond),
					float64(commitTime)/float64(time.Millisecond),
					float64(info.Flush["FlushTime"].LastFlushTime)/float64(time.Millisecond),
					float64(info.Flush["ChecksMetricSampleFlushTime"].LastFlushTime)/float64(time.Millisecond))

				plotRes += fmt.Sprintf(" %f", float64(genTime)/float64(time.Millisecond))
			}
		}

		fmt.Printf("-- %d Events ---\n", nbPoint)
		for _, nbPoint := range nbPoints {
			genTime := generateEvent(nbPoint, sender)
			start := time.Now()
			sender.Commit()
			commitTime := time.Since(start)
			info = report(agg, flush, info, "EventFlushTime")
			fmt.Printf("sent %d Events in %f and commited in %f flush in %f serialize in %f\n",
				nbPoint,
				float64(genTime)/float64(time.Millisecond),
				float64(commitTime)/float64(time.Millisecond),
				float64(info.Flush["FlushTime"].LastFlushTime)/float64(time.Millisecond),
				float64(info.Flush["EventFlushTime"].LastFlushTime)/float64(time.Millisecond))
			plotRes += fmt.Sprintf(" %f", float64(genTime)/float64(time.Millisecond))
		}

		fmt.Printf("-- %d ServiceChecks ---\n", nbPoint)
		for _, nbPoint := range nbPoints {
			genTime := generateServiceCheck(nbPoint, sender)
			start := time.Now()
			sender.Commit()
			commitTime := time.Since(start)
			info = report(agg, flush, info, "ServiceCheckFlushTime")
			fmt.Printf("sent %d Service Checks in %f and commited in %f flush in %f serialize in %f\n",
				nbPoint,
				float64(genTime)/float64(time.Millisecond),
				float64(commitTime)/float64(time.Millisecond),
				float64(info.Flush["FlushTime"].LastFlushTime)/float64(time.Millisecond),
				float64(info.Flush["ServiceCheckFlushTime"].LastFlushTime)/float64(time.Millisecond))
			plotRes += fmt.Sprintf(" %f", float64(genTime)/float64(time.Millisecond))
		}

	}

	return plotRes
}
