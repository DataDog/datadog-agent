// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"expvar"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	datadog "gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

const (
	tMode = 1
	pMode = 2
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var (
	mode       = flag.Int("mode", 1, "1: duration, 2: packets.")
	dur        = flag.Int("dur", 60, "duration for the test in seconds.")
	num        = flag.Int("num", 10000, "number of packets to submit.")
	pps        = flag.Int("pps", 1000, "packets per second.")
	pad        = flag.Int("pad", 2, "tag padding - determines packet size.")
	ser        = flag.Int("ser", 10, "number of distinct series.")
	inc        = flag.Int("inc", 1000, "pps increments per iteration.")
	rnd        = flag.Bool("rnd", false, "random series.")
	brk        = flag.Bool("brk", false, "find breaking point.")
	apiKey     = flag.String("api-key", "", "if set, results will be push to datadog.")
	branchName = flag.String("branch", "", "Add a 'branch' tag to every metrics equal to the value given.")
)

type forwarderBenchStub struct {
	received      uint64
	receivedBytes uint64
}

func (f *forwarderBenchStub) reset() {
	f.received = 0
	f.receivedBytes = 0
}

func (f *forwarderBenchStub) computeStats(payloads transaction.BytesPayloads) {
	for _, payload := range payloads {
		f.received++
		f.receivedBytes += uint64(len(payload.GetContent()))
	}
}

func (f *forwarderBenchStub) Start() error {
	return nil
}

func (f *forwarderBenchStub) Stop() {
	return
}

func (f *forwarderBenchStub) SubmitV1Series(payloads transaction.BytesPayloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitV1Intake(payloads transaction.BytesPayloads, extraHeaders http.Header, priority transaction.Priority) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitV1CheckRuns(payloads transaction.BytesPayloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitSeries(payloads transaction.BytesPayloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitSketchSeries(payloads transaction.BytesPayloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitHostMetadata(payloads transaction.BytesPayloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitMetadata(payloads transaction.BytesPayloads, extraHeaders http.Header, priority transaction.Priority) error {
	f.computeStats(payloads)
	return nil
}

// NewStatsdGenerator returns a generator server
// We could use datadog-go, but I want as little overhead as possible.
func NewStatsdGenerator(uri string) (*net.UDPConn, error) {
	serverAddr, addrErr := net.ResolveUDPAddr("udp", uri)
	if addrErr != nil {
		return nil, fmt.Errorf("dogstatsd: can't ResolveUDPAddr %s: %v", uri, addrErr)
	}

	cliAddr, addrErr := net.ResolveUDPAddr("udp", "localhost:0")
	if addrErr != nil {
		return nil, fmt.Errorf("dogstatsd: can't ResolveUDPAddr %s: %v", uri, addrErr)
	}

	c, err := net.DialUDP("udp", cliAddr, serverAddr)
	if err != nil {
		return nil, fmt.Errorf("dogstatsd: unable to establish connection %s: %v", uri, err)
	}

	return c, nil
}

func initLogging() error {
	err := config.SetupLogger(config.LoggerName("test"), "info", "", "", false, true, false)
	if err != nil {
		return fmt.Errorf("Unable to initiate logger: %s", err)
	}

	return nil
}

// format a message from its name, value, tags and rate.  Also adds global
// namespace and tags.
func buildPayload(name string, value interface{}, suffix []byte, tags []string, rate float64) string {
	var buf bytes.Buffer
	buf.WriteString(name)
	buf.WriteString(":")

	switch val := value.(type) {
	case float64:
		buf.Write(strconv.AppendFloat([]byte{}, val, 'f', 6, 64))

	case int64:
		buf.Write(strconv.AppendInt([]byte{}, val, 10))

	case string:
		buf.WriteString(val)

	default:
		// do nothing
	}
	buf.Write(suffix)

	buf.WriteString(`|@`)
	buf.WriteString(strconv.FormatFloat(rate, 'f', -1, 64))

	// let's do tags
	buf.WriteString("|#")
	buf.WriteString(tags[0])
	for _, tag := range tags[1:] {
		buf.WriteString(",")
		buf.WriteString(tag)
	}

	return buf.String()
}

func submitPacket(buf []byte, conn *net.UDPConn) error {
	_, err := conn.Write(buf)
	return err
}

func randomString(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func createMetric(value float64, tags []string, name string, t int64) datadog.Metric {
	unit := "package"
	metricType := "gauge"
	hostname, _ := os.Hostname()

	return datadog.Metric{
		Metric: &name,
		Points: []datadog.DataPoint{{float64(t), value}},
		Type:   &metricType,
		Host:   &hostname,
		Tags:   tags,
		Unit:   &unit,
	}
}

func main() {
	mockConfig := config.Mock(nil)

	if err := InitLogging("info"); err != nil {
		log.Infof("Unable to replace logger, default logging will apply (highly verbose): %s", err)
	}
	defer log.Flush()

	log.Infof("starting benchmarking...")
	flag.Parse()
	f := &forwarderBenchStub{
		received:      0,
		receivedBytes: 0,
	}

	var wg sync.WaitGroup

	mockConfig.Set("dogstatsd_stats_enable", true)
	mockConfig.Set("dogstatsd_stats_buffer", 100)
	s := serializer.NewSerializer(f, nil)
	aggr := aggregator.NewBufferedAggregator(s, nil, "localhost", aggregator.DefaultFlushInterval)
	statsd, err := dogstatsd.NewServer(aggr.GetBufferedChannels(), false)
	if err != nil {
		log.Errorf("Problem allocating dogstatsd server: %s", err)
		return
	}
	defer statsd.Stop()

	uri := fmt.Sprintf("localhost:%d", config.Datadog.GetInt("dogstatsd_port"))
	generator, err := NewStatsdGenerator(uri)
	if err != nil {
		log.Errorf("Problem allocating statistics generator: %s", err)
		return
	}
	defer generator.Close()

	// Get memory stats from expvar:
	memstatsFunc := expvar.Get("memstats").(expvar.Func)
	memstats := memstatsFunc().(runtime.MemStats)

	if statsd.Statistics != nil {

		iteration := 0
		sent := uint64(0)
		processed := uint64(0)
		quitGenerator := make(chan bool)
		quitStatter := make(chan bool)

		for ok := true; ok; ok = (*brk && processed == sent) {
			rate := (*pps) + iteration*(*inc)
			ticker := time.NewTicker(time.Second / time.Duration(rate))

			wg.Add(1)
			go func() {
				sent = 0
				target := uint64(*num)
				var buf bytes.Buffer
				var packets []string

				defer wg.Done()
				if !(*rnd) {
					packets = make([]string, *ser)
					for i := range packets {
						packets[i] = buildPayload("foo.bar", rand.Int63n(1000), []byte("|g"), []string{RandomString(*pad)}, 1)
					}
				}

				for range ticker.C {
					select {
					case <-quitGenerator:
						quitStatter <- true
						return
					default:
						// Do other stuff
						var err error
						if *rnd {
							buf.Reset()
							buf.WriteString("foo.")
							buf.WriteString(RandomString(*ser))

							err = submitPacket([]byte(buildPayload(buf.String(), rand.Int63n(1000), []byte("|g"), []string{RandomString(*pad)}, 2)), generator)
						} else {
							err = submitPacket([]byte(packets[rand.Int63n(int64(*ser))]), generator)
						}
						if err != nil {
							log.Warnf("Problem sending packet: %v", err)
						}
						if sent++; (*mode == pMode) && (sent == target) {
							quitStatter <- true
							return
						}
					}
				}
			}()

			wg.Add(1)
			go func() {
				log.Infof("[stats] starting stats reader")
				processed = 0
				quit := false
				tickChan := time.NewTicker(time.Second).C
				defer wg.Done()

				for range tickChan {
					select {
					case <-quitStatter:
						quit = true
					case v := <-statsd.Statistics.Aggregated:
						log.Infof("[stats] [mem: %v] processed %v packets @%v ", memstats.Alloc, v.Val, v.Ts)
						if quit && v.Val == 0 {
							return
						}
						processed += uint64(v.Val)
					default:
						log.Infof("[stats] no statistics were available.")
					}
				}
			}()

			// if in timed mode: sleep to quit.
			if *mode == tMode {
				time.Sleep(time.Second * time.Duration(*dur))
				quitGenerator <- true
			}

			wg.Wait()
			ticker.Stop()
			log.Infof("[generator] submit on packet every: %v", time.Second/time.Duration(rate))
			log.Infof("[generator] rate for iteration: %v", rate)
			log.Infof("[generator] pps for iteration: %v", float64(processed)/float64(*dur))
			log.Infof("[generator] packets submitted: %v", sent)
			log.Infof("[dogstatsd] packets processed: %v", processed)
			log.Infof("[forwarder stats] packets received: %v", f.received)
			log.Infof("[forwarder stats] bytes received: %v", f.receivedBytes)

			if *apiKey != "" && *brk && processed != sent {
				log.Infof("Pushing results to DataDog backend")

				t := time.Now().Unix()
				client := datadog.NewClient(*apiKey, "")
				tags := []string{}
				if *branchName != "" {
					log.Infof("Adding branchName to tags")
					tags = []string{fmt.Sprintf("branch:%s", *branchName)}
				}

				results := []datadog.Metric{}
				results = append(results, createMetric(float64(rate), tags, "benchmark.dogstatsd.rate.loss", t))
				results = append(results, createMetric(float64(processed)/float64(*dur), tags, "benchmark.dogstatsd.pps.loss", t))

				err := client.PostMetrics(results)
				if err != nil {
					log.Errorf("Could not post metrics: %s", err)
				}
			}

			f.reset()
			iteration++
		}
	}
}
