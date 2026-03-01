// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package dns

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// NewReverseDNS returns a stub ReverseDNS for Darwin
// DNS monitoring is not yet implemented on macOS
func NewReverseDNS(_ *config.Config, _ telemetry.Component) (ReverseDNS, error) {
	return NewNullReverseDNS(), nil
}
