// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	_ "expvar"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/serializer"

	"gopkg.in/zorkian/go-datadog-api.v2"
)

var (
	points = flag.String("points",
		"100,1000,10000,50000,100000",
		"comma-separated list of number of points to create per series.")

	series = flag.String("series",
		"1,10,100",
		"comma-separated list of number of series to create per metrics.")

	jsonOutput = flag.Bool("json",
		false,
		"if set, the results will be output in JSON.")

	apiKey = flag.String("api-key",
		"",
		"if set, results will be push to datadog.")

	logLevel = flag.String("log-level",
		"info",
		"Silence the default output.")

	branchName = flag.String("branch",
		"",
		"Add a 'branch' tag to every metrics equal to the value given.")

	memory = flag.Bool("memory",
		false,
		"should we run the memory benchmark.")

	memips = flag.Int("ips",
		1000,
		"number of iterations per second (best-effort).")

	duration = flag.Int("duration",
		60,
		"duration per second.")

	flushIval = flag.Int64("flush_ival",
		int64(aggregator.DefaultFlushInterval/time.Second),
		"Flush interval for aggregator, in seconds")

	agg   *aggregator.BufferedAggregator
	flush = make(chan time.Time)
)

type forwarderBenchStub struct{}

func (f *forwarderBenchStub) Start() error { return nil }
func (f *forwarderBenchStub) Stop()        {}
func (f *forwarderBenchStub) SubmitV1Series(payloads transaction.BytesPayloads, extraHeaders http.Header) error {
	return nil
}
func (f *forwarderBenchStub) SubmitV1Intake(payloads transaction.BytesPayloads, extraHeaders http.Header, priority transaction.Priority) error {
	return nil
}
func (f *forwarderBenchStub) SubmitV1CheckRuns(payloads transaction.BytesPayloads, extraHeaders http.Header) error {
	return nil
}
func (f *forwarderBenchStub) SubmitSeries(payload transaction.BytesPayloads, extraHeaders http.Header) error {
	return nil
}
func (f *forwarderBenchStub) SubmitSketchSeries(payload transaction.BytesPayloads, extraHeaders http.Header) error {
	return nil
}
func (f *forwarderBenchStub) SubmitHostMetadata(payload transaction.BytesPayloads, extraHeaders http.Header) error {
	return nil
}
func (f *forwarderBenchStub) SubmitMetadata(payload transaction.BytesPayloads, extraHeaders http.Header, priority transaction.Priority) error {
	return nil
}

type aggregatorStats struct {
	Flush map[string]aggregator.Stats
}

type stats struct {
	Aggregator aggregatorStats `json:"aggregator"`
}

func getExpvarJSON() (*aggregatorStats, error) {
	resp, err := http.Get("http://127.0.0.1:5000/debug/vars")
	if err != nil {
		log.Errorf("could not contact expvar server: %s", err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	res := stats{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		log.Errorf("could not load json: %s", err)
		return nil, err
	}
	return &res.Aggregator, nil
}

func waitForAggregatorEmptyQueue() {
	// waiting for the aggregator to consume every event
	for agg.IsInputQueueEmpty() == false {
		log.Debug("Queue is not empty, waiting a 0.2s")
		time.Sleep(10 * time.Millisecond)
	}
}

func report(lastInfo *aggregatorStats, waitingKey string) *aggregatorStats {
	// waiting for the aggregator to consume every event
	for agg.IsInputQueueEmpty() == false {
		log.Debug("Queue is not empty, waiting a 0.2s")
		time.Sleep(200 * time.Millisecond)
	}
	flush <- time.Now()

	i := 0
	for {
		stats, err := getExpvarJSON()
		if err != nil {
			log.Criticalf("got error from getExpvarJSON: %v", err)
		}

		if lastInfo != nil && lastInfo.Flush[waitingKey].FlushIndex == stats.Flush[waitingKey].FlushIndex {
			time.Sleep(200 * time.Millisecond)
			i++

			// Sometime the flush event was handle before the commit message was finished: resending a flush
			if i > 10 {
				flush <- time.Now()
			}

			continue
		}
		return stats
	}
}

func setupLogger(logLevel string) error {
	configTemplate := `<seelog minlevel="%s">
    <outputs formatid="common"><console/></outputs>
    <formats>
        <format id="common" format="%%LEVEL | (%%RelFile:%%Line) | %%Msg%%n"/>
    </formats>
</seelog>`
	config := fmt.Sprintf(configTemplate, strings.ToLower(logLevel))

	logger, err := log.LoggerFromConfigAsString(config)
	if err != nil {
		return err
	}
	err = log.ReplaceLogger(logger)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	// go_expvar server
	go http.ListenAndServe("127.0.0.1:5000", http.DefaultServeMux)
	rand.Seed(123)
	flag.Parse()

	if err := setupLogger(*logLevel); err != nil {
		fmt.Printf("could not set loggger: %s", err)
		return
	}
	defer log.Flush()

	if *branchName == "" {
		log.Criticalf("Error: '-branch' parameter is mandatory")
		return
	}

	SetHostname("foo")

	f := &forwarderBenchStub{}
	s := serializer.NewSerializer(f, nil)

	agg = aggregator.NewBufferedAggregator(s, nil, "hostname", time.Duration(*flushIval)*time.Second)

	aggregator.SetDefaultAggregator(agg)
	sender, err := aggregator.GetSender(checkid.ID("benchmark check"))
	if err != nil {
		log.Criticalf("could not get sender: %s", err)
		return
	}

	nbPoints := []int{}
	for _, n := range strings.Split(*points, ",") {
		res, err := strconv.Atoi(n)
		if err != nil {
			log.Errorf("Could not parse 'points' arguments '%s': %s", n, err)
			return
		}
		nbPoints = append(nbPoints, res)
	}
	nbSeries := []int{}
	for _, n := range strings.Split(*series, ",") {
		res, err := strconv.Atoi(n)
		if err != nil {
			fmt.Printf("Could not parse 'series' arguments '%s': %s", n, err)
			return
		}
		nbSeries = append(nbSeries, res)
	}

	agg.TickerChan = flush

	//warm up
	generateMetrics(1, 1, sender.Gauge)
	generateEvent(1, sender)
	generateServiceCheck(1, sender)
	sender.Commit()

	var results []datadog.Metric
	if *memory {
		results = benchmarkMemory(agg, sender, nbPoints, nbSeries, *memips, *duration, *branchName)
	} else {
		startInfo := report(nil, "")

		log.Infof("Starting benchmark with %v series of %v points.\n\n", nbSeries, nbPoints)
		results = benchmarkMetrics(nbSeries, nbPoints, sender, startInfo, *branchName)
	}

	if *jsonOutput {
		data, err := json.Marshal(results)
		if err != nil {
			fmt.Printf("Error serializing results to JSON: %s\n", err)
		} else {
			fmt.Println(string(data))
		}
	}

	if *apiKey != "" {
		log.Infof("Pushing results to DataDog backend")
		pushMetricsToDatadog(*apiKey, results)
	} else {
		log.Infof("No API key provided: no results was push to the DataDog backend")
	}
}
