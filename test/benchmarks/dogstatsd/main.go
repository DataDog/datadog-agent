package main

import (
	"bytes"
	"expvar"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	log "github.com/cihub/seelog"
)

const (
	tMode = 1
	pMode = 2
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var (
	mode = flag.Int("mode", 1, "1: duration, 2: packets.")
	dur  = flag.Int("dur", 60, "duration for the test in seconds.")
	num  = flag.Int("num", 10000, "number of packets to submit.")
	pps  = flag.Int("pps", 1000, "packets per second.")
	pad  = flag.Int("pad", 2, "tag padding - determines packet size.")
	ser  = flag.Int("ser", 10, "number of distinct series.")
	inc  = flag.Int("inc", 1000, "pps increments per iteration.")
	rnd  = flag.Bool("rnd", false, "random series.")
	brk  = flag.Bool("brk", false, "find breaking point.")
)

type forwarderBenchStub struct {
	received      uint64
	receivedBytes uint64
}

func (f *forwarderBenchStub) reset() {
	f.received = 0
	f.receivedBytes = 0
}

func (f *forwarderBenchStub) computeStats(payload *[]byte) {
	f.received++
	f.receivedBytes += uint64(len(*payload))
}
func (f *forwarderBenchStub) Start() error {
	return nil
}
func (f *forwarderBenchStub) Stop() {
	return
}
func (f *forwarderBenchStub) SubmitV1Series(apiKey string, payload *[]byte) error {
	f.computeStats(payload)
	return nil
}
func (f *forwarderBenchStub) SubmitV1Intake(apiKey string, payload *[]byte) error {
	f.computeStats(payload)
	return nil
}
func (f *forwarderBenchStub) SubmitV1CheckRuns(apiKey string, payload *[]byte) error {
	f.computeStats(payload)
	return nil
}
func (f *forwarderBenchStub) SubmitV2Series(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}
func (f *forwarderBenchStub) SubmitV2Events(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}
func (f *forwarderBenchStub) SubmitV2CheckRuns(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}
func (f *forwarderBenchStub) SubmitV2HostMeta(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}
func (f *forwarderBenchStub) SubmitV2GenericMeta(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
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
	err := config.SetupLogger("info", "")
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

func main() {
	if err := initLogging(); err != nil {
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

	config.Datadog.Set("dogstatsd_stats_enable", true)
	config.Datadog.Set("dogstatsd_stats_buffer", 100)
	aggr := aggregator.InitAggregator(f, "localhost")
	statsd, err := dogstatsd.NewServer(aggr.GetChannels())
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
						packets[i] = buildPayload("foo.bar", rand.Int63n(1000), []byte("|g"), []string{randomString(*pad)}, 1)
					}
				}

				for _ = range ticker.C {
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
							buf.WriteString(randomString(*ser))

							err = submitPacket([]byte(buildPayload(buf.String(), rand.Int63n(1000), []byte("|g"), []string{randomString(*pad)}, 2)), generator)
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

				for _ = range tickChan {
					select {
					case <-quitStatter:
						quit = true
					case v := <-statsd.Statistics.Aggregated:
						log.Infof("[stats] [mem: %v] proceesed %v packets @%v ", memstats.Alloc, v.Val, v.Ts)
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
			log.Infof("[generator] rate for iteration in pps: %v", rate)
			log.Infof("[generator] packets submitted: %v", sent)
			log.Infof("[dogstatsd] packets processed: %v", processed)
			log.Infof("[forwarder stats] packets received: %v", f.received)
			log.Infof("[forwarder stats] bytes received: %v", f.receivedBytes)

			f.reset()
			iteration++
		}
	}
}
