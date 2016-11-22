package main

import (
	"hash/fnv"
	"math/rand"
	"strings"
	"time"

	"github.com/PagerDuty/godspeed"
	log "github.com/cihub/seelog"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func sendGauge(gStress, gStats *godspeed.Godspeed, metric string, value float64, tags []string) {
	// err := gStress.Gauge("example.stat", 1, tags)
	err := gStress.Gauge("example.stat", 1, nil)

	if err != nil {
		log.Errorf("error sending gauge")
	}

	gStats.Send("datadog.agent.dogstatsd_bench.sent", "c", 1, 0.01, []string{"metric_type:g"})
	metricTag := "metric_name:" + metric
	context := append(tags, metricTag)
	h := fnv.New32()
	h.Write([]byte(strings.Join(context, "|")))
	hash := h.Sum32()

	gStats.Set("datadog.agent.dogstatsd_bench.context", float64(hash), nil)
}

func randomString() string {
	b := make([]byte, 2)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func main() {
	log.Infof("starting")
	gStress, err := godspeed.New("127.0.0.1", 8126, false)
	if err != nil {
		log.Errorf("ERROR")
	}
	defer gStress.Conn.Close()

	gStats, err := godspeed.New("127.0.0.1", 8125, false)
	if err != nil {
		log.Errorf("ERROR")
	}
	defer gStats.Conn.Close()

	for {
		sendGauge(gStress, gStats, "example.stat", 1, []string{"key:value", randomString()})
		time.Sleep(1 * time.Second)
	}
}
