// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package main

import (
	"bufio"
	"bytes"
	"expvar"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/test/util"
	log "github.com/cihub/seelog"
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
	buf        = flag.Bool("buf", false, "buffered socket writes")
	rnd        = flag.Bool("rnd", false, "random series.")
	brk        = flag.Bool("brk", false, "find breaking point.")
	snd        = flag.Bool("snd", false, "just send - don't start receiver (useful for out-of-band testing).")
	apiKey     = flag.String("api-key", "", "if set, results will be push to datadog.")
	branchName = flag.String("branch", "", "Add a 'branch' tag to every metrics equal to the value given.")
	dst        = flag.String("dst", "localhost:8125", "destination address")
)

type forwarderBenchStub struct {
	received      uint64
	receivedBytes uint64
}

func (f *forwarderBenchStub) reset() {
	f.received = 0
	f.receivedBytes = 0
}

func (f *forwarderBenchStub) computeStats(payloads forwarder.Payloads) {
	for _, payload := range payloads {
		f.received++
		f.receivedBytes += uint64(len(*payload))
	}
}

func (f *forwarderBenchStub) Start() error {
	return nil
}

func (f *forwarderBenchStub) Stop() {
	return
}

func (f *forwarderBenchStub) SubmitV1Series(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitV1Intake(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitV1CheckRuns(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitSeries(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitEvents(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitServiceChecks(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitSketchSeries(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitHostMetadata(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}
func (f *forwarderBenchStub) SubmitMetadata(payloads forwarder.Payloads, extraHeaders http.Header) error {
	f.computeStats(payloads)
	return nil
}

// We could use datadog-go, but I want as little overhead as possible.
func newStatsdGeneratorSocket(uri string) (*net.UDPConn, error) {

	serverAddr, addrErr := net.ResolveUDPAddr("udp", uri)
	if addrErr != nil {
		return nil, fmt.Errorf("dogstatsd: can't ResolveUDPAddr %s: %v", uri, addrErr)
	}

	localhost := "localhost:0"
	if strings.Contains(uri, ":") { //IPv6
		localhost = "[::1]:0"
	}

	cliAddr, addrErr := net.ResolveUDPAddr("udp", localhost)
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
	err := config.SetupLogger("info", "", "", false, false, "", true)
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

func submitPacket(buf []byte, w io.Writer) error {
	_, err := w.Write(buf)
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

func generate(num, ser, pad int, rate int64, rnd, snd bool, w io.Writer,
	wg *sync.WaitGroup, quitStat, quitGen chan bool, sentC chan uint64) {
	sent := uint64(0)
	var buf bytes.Buffer
	var packets []string

	defer wg.Done()
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	if !rnd {
		packets = make([]string, ser)
		for i := range packets {
			packets[i] = buildPayload("foo.bar", rand.Int63n(1000), []byte("|g"), []string{util.RandomString(pad)}, 1)
		}
	}

	for {
		select {
		case <-quitGen:
			ticker.Stop()
			if !snd {
				quitStat <- true
			}
		case <-ticker.C:
			// Do other stuff
			var err error
			if rnd {
				buf.Reset()
				buf.WriteString("foo.")
				buf.WriteString(util.RandomString(ser))

				err = submitPacket([]byte(buildPayload(buf.String(), rand.Int63n(1000), []byte("|g"), []string{util.RandomString(pad)}, 2)), w)
			} else {
				err = submitPacket([]byte(packets[rand.Int63n(int64(ser))]), w)
			}
			if err != nil {
				log.Warnf("Problem sending packet: %v", err)
			}
			if sent++; (*mode == pMode) && (sent == uint64(num)) {
				ticker.Stop()
				if !snd {
					quitStat <- true
				}
			}
			continue
		}
		break
	}
	sentC <- sent
}

func main() {
	if err := util.InitLogging("info"); err != nil {
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

	var statsd *dogstatsd.Server
	var err error
	if !*snd {
		config.Datadog.Set("dogstatsd_stats_enable", true)
		config.Datadog.Set("dogstatsd_stats_buffer", 100)
		s := &serializer.Serializer{Forwarder: f}
		aggr := aggregator.InitAggregator(s, "localhost")
		statsd, err = dogstatsd.NewServer(aggr.GetChannels())
		if err != nil {
			log.Errorf("Problem allocating dogstatsd server: %s", err)
			return
		}
		defer statsd.Stop()
	}

	generator, err := newStatsdGeneratorSocket(*dst)
	if err != nil {
		log.Errorf("Problem allocating statistics generator: %s", err)
		return
	}
	defer generator.Close()

	var pktWriter io.Writer
	if *buf {
		pktWriter = bufio.NewWriter(generator)
	} else {
		pktWriter = generator
	}

	// Get memory stats from expvar:
	memstatsFunc := expvar.Get("memstats").(expvar.Func)
	memstats := memstatsFunc().(runtime.MemStats)

	if *snd || statsd.Statistics != nil {

		iteration := 0
		sent := uint64(0)
		processed := uint64(0)
		quitGenerator := make(chan bool)
		quitStatter := make(chan bool)
		sentC := make(chan uint64)

		for ok := true; ok; ok = (*brk && processed == sent) {
			rate := (*pps) + iteration*(*inc)

			wg.Add(1)
			go generate(*num, *ser, *pad, int64(rate), *rnd, *snd, pktWriter, &wg, quitStatter, quitGenerator, sentC)

			if !*snd {
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
			}

			// if in timed mode: sleep to quit.
			if *mode == tMode {
				time.Sleep(time.Second * time.Duration(*dur))
				quitGenerator <- true
			}

			sent := <-sentC
			wg.Wait()

			log.Infof("[generator] submit on packet every: %v", time.Second/time.Duration(rate))
			log.Infof("[generator] rate for iteration: %v", rate)
			log.Infof("[generator] pps for iteration: %v", float64(processed)/float64(*dur))
			log.Infof("[generator] packets submitted: %v", sent)
			if !*snd {
				log.Infof("[dogstatsd] packets processed: %v", processed)
				log.Infof("[forwarder stats] packets received: %v", f.received)
				log.Infof("[forwarder stats] bytes received: %v", f.receivedBytes)
			}

			if *apiKey != "" && !*snd && *brk && processed != sent {
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
