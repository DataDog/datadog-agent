// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"fmt"

	config "github.com/DataDog/datadog-agent/comp/netflow/config/def"
	"github.com/DataDog/datadog-agent/comp/netflow/dpi"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/netflowstate"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/templates/memory"

	"go.uber.org/atomic"

	"github.com/netsampler/goflow2/utils"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
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
func StartFlowRoutine(
	flowType common.FlowType,
	hostname string,
	port uint16,
	workers int,
	namespace string,
	fieldMappings []config.Mapping,
	enableBiflowParsing bool,
	templateCacheMaxSize int,
	appCache dpi.Cache,
	flowInChan chan *common.Flow,
	logger log.Component,
	atomicErr *atomic.String,
	listenerFlowCount *atomic.Int64) (*FlowStateWrapper, error) {
	var flowState FlowRunnableState

	formatDriver := NewAggregatorFormatDriver(flowInChan, namespace, listenerFlowCount, enableBiflowParsing, appCache != nil)
	goflowLogger := &GoflowLoggerAdapter{logger}

	switch flowType {
	case common.TypeNetFlow9, common.TypeIPFIX:
		state := netflowstate.NewStateNetFlow(fieldMappings, enableBiflowParsing, namespace, appCache)
		state.Format = formatDriver
		state.Logger = goflowLogger
		state.TemplateSystem = memory.New(templateCacheMaxSize)
		flowState = state
	case common.TypeSFlow5:
		state := utils.NewStateSFlow()
		state.Format = formatDriver
		state.Logger = goflowLogger
		flowState = state
	case common.TypeNetFlow5:
		state := utils.NewStateNFLegacy()
		state.Format = formatDriver
		state.Logger = goflowLogger
		flowState = state
	default:
		return nil, fmt.Errorf("unknown flow type: %s", flowType)
	}

	go func() {
		err := flowState.FlowRoutine(workers, hostname, int(port), reusePort)
		if err != nil {
			logger.Errorf("Error listening to %s: %s", flowType, err)
			atomicErr.Store(err.Error())
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
