// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

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

// ParseEventsFile reads a file containing JSON-encoded events recorded with the
// `gpu/debug/collect-events` endpoint from system-probe (in a format of one per
// line) and returns a slice of events.
func ParseEventsFile(path string) ([]any, error) {
	var events []any

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

		events = append(events, completeEvent)
	}

	return events, nil
}

// GetEventTimestamp returns the timestamp of a gpuebpf event by casting it to the
// appropriate type and extracting the timestamp field. Not the best way to do this but
// considering it's just testutil code, it works well enough rather than modifying the ebpf package
func GetEventTimestamp(ev any) uint64 {
	switch e := ev.(type) {
	case *ebpf.CudaKernelLaunch:
		return e.Header.Ktime_ns
	case *ebpf.CudaMemEvent:
		return e.Header.Ktime_ns
	case *ebpf.CudaSync:
		return e.Header.Ktime_ns
	case *ebpf.CudaSetDeviceEvent:
		return e.Header.Ktime_ns
	default:
		return 0
	}
}

// EventToString converts an event to a human-readable string based on its type
func EventToString(ev any, lastTimestamp uint64) (string, error) {
	switch e := ev.(type) {
	case *ebpf.CudaKernelLaunch:
		return fmt.Sprintf("[%s] kernel launch addr 0x%X", headerToString(&e.Header, lastTimestamp), e.Kernel_addr), nil
	case *ebpf.CudaMemEvent:
		memName := "allocation"
		if ebpf.CudaMemEventType(e.Type) == ebpf.CudaMemFree {
			memName = "free"
		}
		return fmt.Sprintf("[%s] memory %s addr 0x%X size %d", headerToString(&e.Header, lastTimestamp), memName, e.Addr, e.Size), nil
	case *ebpf.CudaSync:
		return fmt.Sprintf("[%s] sync event", headerToString(&e.Header, lastTimestamp)), nil
	case *ebpf.CudaSetDeviceEvent:
		return fmt.Sprintf("[%s] set device event device %d", headerToString(&e.Header, lastTimestamp), e.Device), nil
	default:
		return "", fmt.Errorf("unsupported event type: %T", e)
	}
}

func headerToString(header *ebpf.CudaEventHeader, lastTimestamp uint64) string {
	pid := header.Pid_tgid >> 32
	tid := header.Pid_tgid & 0xffffffff

	var diffStr string
	if lastTimestamp != 0 {
		diff := header.Ktime_ns - lastTimestamp
		diffStr = fmt.Sprintf(" (%+6.3fms)", float64(diff)/1e6)
	}

	return fmt.Sprintf("PID/TID: %d/%d | STR: %d | T %d%s", pid, tid, header.Stream_id, header.Ktime_ns, diffStr)
}
