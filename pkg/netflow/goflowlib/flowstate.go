// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"fmt"

	"github.com/netsampler/goflow2/utils"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

// setting reusePort to false since not expected to be useful
// more info here: https://stackoverflow.com/questions/14388706/how-do-so-reuseaddr-and-so-reuseport-differ
const reusePort = false

// FlowStateWrapper is a wrapper for StateNetFlow/StateSFlow/StateNFLegacy to provide additional info like hostname/port
type FlowStateWrapper struct {
	State    FlowRunnableState
	Hostname string
	Port     uint16
}

// FlowRunnableState provides common interface for StateNetFlow/StateSFlow/StateNFLegacy/etc
type FlowRunnableState interface {
	// FlowRoutine starts flow processing workers
	FlowRoutine(workers int, addr string, port int, reuseport bool) error

	// Shutdown trigger shutdown of the flow processing workers
	Shutdown()
}

// StartFlowRoutine starts one of the goflow flow routine depending on the flow type
func StartFlowRoutine(flowType common.FlowType, hostname string, port uint16, workers int, namespace string, flowInChan chan *common.Flow) (*FlowStateWrapper, error) {
	var flowState FlowRunnableState

	formatDriver := NewAggregatorFormatDriver(flowInChan, namespace)
	logger := GetLogrusLevel()

	switch flowType {
	case common.TypeNetFlow9, common.TypeIPFIX:
		flowState = &utils.StateNetFlow{
			Format: formatDriver,
			Logger: logger,
		}
	case common.TypeSFlow5:
		flowState = &utils.StateSFlow{
			Format: formatDriver,
			Logger: logger,
		}
	case common.TypeNetFlow5:
		flowState = &utils.StateNFLegacy{
			Format: formatDriver,
			Logger: logger,
		}
	default:
		return nil, fmt.Errorf("unknown flow type: %s", flowType)
	}

	go func() {
		err := flowState.FlowRoutine(workers, hostname, int(port), reusePort)
		if err != nil {
			log.Errorf("Error listening to %s: %s", flowType, err)
		}
	}()
	return &FlowStateWrapper{
		State:    flowState,
		Hostname: hostname,
		Port:     port,
	}, nil
}

// Shutdown is a wrapper for StateNetFlow/StateSFlow/StateNFLegacy Shutdown method
func (s *FlowStateWrapper) Shutdown() {
	s.State.Shutdown()
}
