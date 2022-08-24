// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package common

import (
	"fmt"
)

// FlowType represent the flow protocol (netflow5,netflow9,ipfix, sflow, etc)
type FlowType string

// Flow Types
const (
	TypeIPFIX    FlowType = "ipfix"
	TypeSFlow5   FlowType = "sflow5"
	TypeNetFlow5 FlowType = "netflow5"
	TypeNetFlow9 FlowType = "netflow9"
	TypeUnknown  FlowType = "unknown"
)

// FlowTypeDetails contain list of valid FlowTypeDetail
var FlowTypeDetails = map[FlowType]FlowTypeDetail{
	TypeIPFIX: {
		name:        TypeIPFIX,
		defaultPort: uint16(4739),
	},
	TypeSFlow5: {
		name:        TypeSFlow5,
		defaultPort: uint16(6343),
	},
	TypeNetFlow5: {
		name:        TypeNetFlow5,
		defaultPort: uint16(2055),
	},
	TypeNetFlow9: {
		name:        TypeNetFlow9,
		defaultPort: uint16(2055),
	},
}

// FlowTypeDetail represent the flow protocol (netflow5,netflow9,ipfix, sflow, etc)
type FlowTypeDetail struct {
	name        FlowType
	defaultPort uint16
}

// Name returns the flow type name
func (f FlowTypeDetail) Name() FlowType {
	return f.name
}

// DefaultPort returns the default port
func (f FlowTypeDetail) DefaultPort() uint16 {
	return f.defaultPort
}

// GetFlowTypeByName search FlowTypeDetail by name
func GetFlowTypeByName(name FlowType) (FlowTypeDetail, error) {
	detail, ok := FlowTypeDetails[name]
	if !ok {
		return FlowTypeDetail{}, fmt.Errorf("flow type `%s` is not valid", name)
	}
	return detail, nil
}

// GetAllFlowTypes returns all flow names
func GetAllFlowTypes() []FlowType {
	var flowTypes []FlowType
	for flowType := range FlowTypeDetails {
		flowTypes = append(flowTypes, flowType)
	}
	return flowTypes
}
