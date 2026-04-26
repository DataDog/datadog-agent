// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !ncm

// Package com_datadoghq_remoteaction_networkconfigmanagement provides PAR actions for network configuration management (stub)
package com_datadoghq_remoteaction_networkconfigmanagement

import (
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// NewNetworkConfigManagement returns nil when built without the ncm build tag
func NewNetworkConfigManagement(_ ipc.HTTPClient) types.Bundle {
	return nil
}
