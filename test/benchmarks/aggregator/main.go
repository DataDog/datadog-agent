package main

import (
	"encoding/json"
	_ "expvar"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
)

var (
	points = flag.String("points",
		"100,1000,10000,50000,100000",
		"comma-separated list of number of points to create per series.")

	series = flag.String("series",
		"1,10,100",
		"comma-separated list of number of series to create per metrics.")

	plotFile = flag.String("plot",
		"",
		"if set, the file where to write results to be plot with gnuplot.")
)

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
	body, err := ioutil.ReadAll(resp.Body)

	res := stats{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		log.Errorf("could not load json: %s", err)
		return nil, err
	}
	return &res.Aggregator, nil
}

func report(agg *aggregator.BufferedAggregator, flush chan time.Time, lastInfo *aggregatorStats, waitingKey string) *aggregatorStats {
	// waiting for the aggregator to consume every event
	for agg.IsInputQueueEmpty() == false {
	}
	flush <- time.Now()

	for {
		stats, err := getExpvarJSON()
		if err != nil {
			log.Errorf("got error from getExpvarJSON: %v", err)
			log.Flush()
			return nil
		}

		if lastInfo != nil && lastInfo.Flush[waitingKey].LastFlushTime == stats.Flush[waitingKey].LastFlushTime {
			log.Info("flush Time is not over, waiting a second")
			time.Sleep(1 * time.Second)
			continue
		}
		return stats
	}
}

func main() {
	// go_expvar server
	go http.ListenAndServe("127.0.0.1:5000", http.DefaultServeMux)
	rand.Seed(123)
	flag.Parse()

	config.SetupLogger("error", "")
	f := forwarder.NewDefaultForwarder(map[string][]string{})

	agg := aggregator.NewBufferedAggregator(f, "benchmark")
	flush := make(chan time.Time)
	agg.TickerChan = flush

	aggregator.SetDefaultAggregator(agg)
	sender, err := aggregator.GetSender(check.ID("benchmark check"))
	if err != nil {
		log.Criticalf("could not get sender: %s", err)
		return
	}

	//warm up
	generateMetrics(1, 1, sender.Gauge)
	generateEvent(1, sender)
	generateServiceCheck(1, sender)
	sender.Commit()
	startInfo := report(agg, flush, nil, "")

	nbPoints := []int{}
	for _, n := range strings.Split(*points, ",") {
		res, err := strconv.Atoi(n)
		if err != nil {
			fmt.Printf("Could not parse 'points' arguments '%s': %s", n, err)
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

	fmt.Printf("Starting benchmark with %v series of %v points.\n\n", nbSeries, nbPoints)
	plotRes := benchmarkMetrics(agg, nbSeries, nbPoints, sender, flush, startInfo)
	if *plotFile != "" {
		ioutil.WriteFile(*plotFile, []byte(plotRes), 0666)
	}
}
