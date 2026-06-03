// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package sds

// ReconfigureOrderType is the type of a reconfiguration order.
type ReconfigureOrderType string

const (
	// DatadogRules triggers a (re)configuration of the SDS scanner with a set
	// of self-contained rules (each rule carries its own pattern and match
	// action).
	DatadogRules ReconfigureOrderType = "datadog_rules"
)

// ReconfigureOrder is used to trigger a reconfiguration of the SDS scanner.
type ReconfigureOrder struct {
	Type   ReconfigureOrderType
	Config []byte
}
