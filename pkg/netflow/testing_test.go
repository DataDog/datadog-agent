// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package netflow

import (
	"encoding/json"
	"fmt"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflowlib"
	"github.com/DataDog/datadog-agent/pkg/netflow/payload"
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

func findEventBySourceDest(events []*message.Message, sourceIP string, destIP string) (payload.FlowPayload, error) {
	for _, event := range events {
		actualFlow := payload.FlowPayload{}
		_ = json.Unmarshal(event.Content, &actualFlow)
		if actualFlow.Source.IP == sourceIP && actualFlow.Destination.IP == destIP {
			return actualFlow, nil
		}
	}
	return payload.FlowPayload{}, fmt.Errorf("no event found that matches `source=%s`, `destination=%s", sourceIP, destIP)
}
