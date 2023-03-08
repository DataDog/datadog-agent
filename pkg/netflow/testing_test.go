// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package netflow

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/netflow/flowaggregator"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
	"time"

	"github.com/DataDog/datadog-agent/pkg/netflow/goflowlib"
)

type dummyFlowProcessor struct {
	receivedMessages chan interface{}
	stopped          bool
}

func (d *dummyFlowProcessor) FlowRoutine(workers int, addr string, port int, reuseport bool) error {
	return utils.UDPStoppableRoutine(make(chan struct{}), "test_udp", func(msg interface{}) error {
		d.receivedMessages <- msg
		return nil
	}, 3, addr, port, false, logrus.StandardLogger())
}

func (d *dummyFlowProcessor) Shutdown() {
	d.stopped = true
}

func replaceWithDummyFlowProcessor(server *Server, port uint16) *dummyFlowProcessor {
	// Testing using a dummyFlowProcessor since we can't test using real goflow flow processor
	// due to this race condition https://github.com/netsampler/goflow2/issues/83
	flowProcessor := &dummyFlowProcessor{}
	listener := server.listeners[0]
	listener.flowState = &goflowlib.FlowStateWrapper{
		State:    flowProcessor,
		Hostname: "abc",
		Port:     port,
	}
	return flowProcessor
}

//func findEventBySourceDest(events []*message.Message, sourceIP string, destIP string) (payload.FlowPayload, error) {
//	for _, event := range events {
//		actualFlow := payload.FlowPayload{}
//		_ = json.Unmarshal(event.Content, &actualFlow)
//		if actualFlow.Source.IP == sourceIP && actualFlow.Destination.IP == destIP {
//			return actualFlow, nil
//		}
//	}
//	return payload.FlowPayload{}, fmt.Errorf("no event found that matches `source=%s`, `destination=%s", sourceIP, destIP)
//}

// WaitEventPlatformEvents waits for timeout and eventually returns the event platform events samples received by the demultiplexer.
func waitEventPlatformEvents(agg *flowaggregator.FlowAggregator, eventType string, minEvents int, timeout time.Duration) (int, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeoutOn := time.Now().Add(timeout)
	var eventsCount int
	for {
		select {
		case <-ticker.C:
			eventsCount = int(agg.GetFlushedFlowCount())
			// this case could always take priority on the timeout case, we have to make sure
			// we've not timeout
			if time.Now().After(timeoutOn) {
				return 0, fmt.Errorf("timeout waitig for events (expected at least %d events but only received %d)", minEvents, eventsCount)
			}

			if eventsCount >= minEvents {
				return eventsCount, nil
			}
		case <-time.After(timeout):
			return 0, fmt.Errorf("timeout waitig for events (expected at least %d events but only received %d)", minEvents, eventsCount)
		}
	}
}
