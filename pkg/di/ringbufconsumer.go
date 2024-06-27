package di

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/DataDog/datadog-agent/pkg/di/eventparser"
	"github.com/DataDog/datadog-agent/pkg/di/ratelimiter"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
)

var (
	bpffs                         string = "/sys/fs/bpf"
	globalEventsRingbufferPinPath string = filepath.Join(bpffs, "events")
)

// startRingbufferConsumer opens the pinned bpf ringbuffer map
func (goDI *GoDI) startRingbufferConsumer() (func(), error) {

	// Open ringbuffer bpffs pinned map
	// start consuming, call callback on each
	m, err := ebpf.LoadPinnedMap(globalEventsRingbufferPinPath, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't load pinned map: %w", err)
	}

	r, err := ringbuf.NewReader(m)
	if err != nil {
		return nil, fmt.Errorf("couldn't set up reader for ringbuffer: %w", err)
	}

	var (
		record ringbuf.Record
		closed bool = false
	)

	closeFunc := func() {
		closed = true
		r.Close()
	}

	// TODO: ensure rate limiters are removed once probes are removed
	rateLimiters := ratelimiter.NewMultiProbeRateLimiter(1.0)
	rateLimiters.SetRate(ditypes.ConfigBPFProbeID, 0)

	go func() {
		for {
			if closed {
				break
			}
			err = r.ReadInto(&record)
			if err != nil {
				log.Printf("couldn't read event off ringbuffer: %s", err.Error())
				continue
			}

			event := eventparser.ParseEvent(record.RawSample, rateLimiters)
			if event == nil {
				continue
			}
			goDI.stats.PIDTriggerCount[event.PID]++
			goDI.stats.ProbeTriggerCount[event.ProbeID]++
			goDI.processEvent(event)
		}
	}()

	return closeFunc, nil
}
