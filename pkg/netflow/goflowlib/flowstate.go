// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"context"
	"fmt"

	"github.com/netsampler/goflow2/decoders/netflow/templates"
	// install the in-memory template manager
	_ "github.com/netsampler/goflow2/decoders/netflow/templates/memory"
	"github.com/netsampler/goflow2/utils"

	"github.com/DataDog/datadog-agent/comp/core/log"

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
func StartFlowRoutine(flowType common.FlowType, hostname string, port uint16, workers int, namespace string, flowInChan chan *common.Flow, logger log.Component) (*FlowStateWrapper, error) {
	var flowState FlowRunnableState

	formatDriver := NewAggregatorFormatDriver(flowInChan, namespace)
	logrusLogger := GetLogrusLevel(logger)
	ctx := context.Background()

	switch flowType {
	case common.TypeNetFlow9, common.TypeIPFIX:
		templateSystem, err := templates.FindTemplateSystem(ctx, "memory")
		if err != nil {
			return nil, fmt.Errorf("goflow template system error flow type: %w", err)
		}
		defer templateSystem.Close(ctx)

		state := utils.NewStateNetFlow()
		state.Format = formatDriver
		state.Logger = logrusLogger
		state.TemplateSystem = templateSystem
		flowState = state
	case common.TypeSFlow5:
		state := utils.NewStateSFlow()
		state.Format = formatDriver
		state.Logger = logrusLogger
		flowState = state
	case common.TypeNetFlow5:
		state := utils.NewStateNFLegacy()
		state.Format = formatDriver
		state.Logger = logrusLogger
		flowState = state
	default:
		return nil, fmt.Errorf("unknown flow type: %s", flowType)
	}

	go func() {
		err := flowState.FlowRoutine(workers, hostname, int(port), reusePort)
		if err != nil {
			logger.Errorf("Error listening to %s: %s", flowType, err)
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
