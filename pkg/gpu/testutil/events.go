// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
)

type partialEvent struct {
	Header ebpf.CudaEventHeader `json:"header"`
}

func parseEventWithType[K any](data []byte) (*K, error) {
	var event K
	err := json.Unmarshal(data, &event)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func parseCompleteEvent(eventType ebpf.CudaEventType, data []byte) (any, error) {
	switch eventType {
	case ebpf.CudaEventTypeKernelLaunch:
		return parseEventWithType[ebpf.CudaKernelLaunch](data)
	case ebpf.CudaEventTypeMemory:
		return parseEventWithType[ebpf.CudaMemEvent](data)
	case ebpf.CudaEventTypeSync:
		return parseEventWithType[ebpf.CudaSync](data)
	case ebpf.CudaEventTypeSetDevice:
		return parseEventWithType[ebpf.CudaSetDeviceEvent](data)
	default:
		return nil, fmt.Errorf("unsupported event type %d", eventType)
	}
}

// EventCollection represents a collection of recorded CUDA events from the system-probe
// module, as returned by the `gpu/debug/collect-events` endpoint. This struct encapsulates methods to
// parse, manipulate and display the events.
type EventCollection struct {
	Events       []any
	firstKtimeNs uint64
	lastKtimeNs  uint64
}

// NewEventCollection reads a file containing JSON-encoded events recorded with the
// `gpu/debug/collect-events` endpoint from system-probe (in a format of one per
// line) and returns a slice of events.
func NewEventCollection(path string) (*EventCollection, error) {
	coll := &EventCollection{}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %v", path, err)
	}

	scanner := bufio.NewScanner(file)

	// Each line is an event
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		// parse the header first
		var event partialEvent
		err := json.Unmarshal([]byte(line), &event)
		if err != nil {
			return nil, fmt.Errorf("cannot parse line %d: %v", lineNumber, err)
		}

		// parse the rest of the event now that we know the event type:
		completeEvent, err := parseCompleteEvent(ebpf.CudaEventType(event.Header.Type), []byte(line))
		if err != nil {
			return nil, fmt.Errorf("cannot parse line %d: %v", lineNumber, err)
		}

		if len(coll.Events) == 0 {
			coll.firstKtimeNs = event.Header.Ktime_ns
		}
		coll.lastKtimeNs = event.Header.Ktime_ns

		coll.Events = append(coll.Events, completeEvent)
	}

	return coll, nil
}

// headerToString converts a CUDA event header to a human-readable string, including relative time
// from the previous event if it's not zero
func (c *EventCollection) headerToString(header *ebpf.CudaEventHeader, prevKtimeNs uint64) string {
	pid := header.Pid_tgid >> 32
	tid := header.Pid_tgid & 0xffffffff

	var diffStr string
	if prevKtimeNs != 0 {
		// Print delta with the previous event, in milliseconds with 3 decimal points
		diff := header.Ktime_ns - prevKtimeNs
		diffStr = fmt.Sprintf(" (%+6.3fms)", float64(diff)/1e6)
	}

	// Output timestamps relative to the start of the first event, it makes it easier to understand
	tsMsec := float64(header.Ktime_ns-c.firstKtimeNs) / 1e6

	return fmt.Sprintf("PID/TID: %d/%d | Stream ID: %d | Time %6.3f%s", pid, tid, header.Stream_id, tsMsec, diffStr)
}

// OutputEvents outputs the events in the collection to the given writer, including a summary of
// grouped events by type for each PID.
func (c *EventCollection) OutputEvents(writer io.Writer) error {
	prevKtimeNs := uint64(0)

	// output events by groups for each PID for the summary later
	groupers := make(map[uint64]*eventGrouper)

	for i, ev := range c.Events {
		var header *ebpf.CudaEventHeader
		var evStr string
		switch e := ev.(type) {
		case *ebpf.CudaKernelLaunch:
			header = &e.Header
			evStr = fmt.Sprintf("kernel launch addr 0x%X", e.Kernel_addr)
		case *ebpf.CudaMemEvent:
			memName := "allocation"
			if ebpf.CudaMemEventType(e.Type) == ebpf.CudaMemFree {
				memName = "free"
			}
			header = &e.Header
			evStr = fmt.Sprintf("memory %s addr 0x%X size %d", memName, e.Addr, e.Size)
		case *ebpf.CudaSync:
			header = &e.Header
			evStr = "sync event"
		case *ebpf.CudaSetDeviceEvent:
			header = &e.Header
			evStr = fmt.Sprintf("set device event device %d", e.Device)
		default:
			return fmt.Errorf("%d: unsupported event type: %T", i, e)
		}

		headerStr := c.headerToString(header, prevKtimeNs)
		prevKtimeNs = header.Ktime_ns

		fmt.Fprintf(writer, "%d: [%s] %s\n", i, headerStr, evStr)

		tsMsec := float64(header.Ktime_ns-c.firstKtimeNs) / 1e6

		pid := header.Pid_tgid >> 32
		if _, ok := groupers[pid]; !ok {
			groupers[pid] = &eventGrouper{}
		}
		groupers[pid].addEvent(ebpf.CudaEventType(header.Type), tsMsec, i)
	}

	// flush the last value
	for _, grouper := range groupers {
		grouper.flushCurrent(0)
	}

	fmt.Fprintf(writer, "\n\nGrouped events:\n")

	for pid, grouper := range groupers {
		fmt.Fprintf(writer, "\n=== PID: %d\n", pid)

		for _, group := range grouper.groups {
			fmt.Fprintf(writer, "%s\n", group)
		}
	}

	return nil
}

// eventGrouper is just a small helper function to have a running count of subsequent events of the same type
type eventGrouper struct {
	groups            []string
	currentEventType  ebpf.CudaEventType
	currentEventCount int
	firstTimestampMs  float64
	lastTimestampMs   float64

	firstEventID int
	lastEventID  int
}

// flushCurrent flushes the current event count to the groups slice
func (g *eventGrouper) flushCurrent(nextTsMs float64) {
	if g.currentEventCount > 0 {
		typeStr := strings.Replace(g.currentEventType.String(), "CudaEventType", "", 1)
		g.groups = append(g.groups, fmt.Sprintf("%13s x %3d: from %.3f to %.3f (lasts %.3fms) | IDs %d -> %d", typeStr, g.currentEventCount, g.firstTimestampMs, g.lastTimestampMs, g.lastTimestampMs-g.firstTimestampMs, g.firstEventID, g.lastEventID))

		// if the last group was too long ago, print a "INACTIVE" group
		timeSinceLast := nextTsMs - g.lastTimestampMs
		if g.lastTimestampMs > 0 && timeSinceLast > 1 {
			g.groups = append(g.groups, fmt.Sprintf("%19s: from %.3f to %.3f (lasts %.3fms)", "== INACTIVE == ", g.lastTimestampMs, nextTsMs, timeSinceLast))
		}
	}
}

// addEvent adds an event to the current group, or starts a new one if the event type is different
func (g *eventGrouper) addEvent(eventType ebpf.CudaEventType, tsMsec float64, evNumber int) {
	if g.currentEventType != eventType {
		g.flushCurrent(tsMsec)
		g.currentEventType = eventType
		g.currentEventCount = 0
		g.firstTimestampMs = tsMsec
		g.firstEventID = evNumber
	}

	g.currentEventCount++
	g.lastTimestampMs = tsMsec
	g.lastEventID = evNumber
}
