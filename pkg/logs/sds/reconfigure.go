// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package sds

type ReconfigureOrderType string

const (
	// StandardRules triggers the storage of a new set of standard rules
	// and reconfigure the internal SDS scanner with an existing user
	// configuration if any.
	StandardRules ReconfigureOrderType = "standard_rules"
	// AgentConfig triggers a reconfiguration of the SDS scanner.
	AgentConfig ReconfigureOrderType = "agent_config"
)

// ReconfigureOrder are used to trigger a reconfiguration
// of the SDS scanner.
type ReconfigureOrder struct {
	Type         ReconfigureOrderType
	Config       []byte
	ResponseChan chan error
}
