// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !linux && !windows

package cpu

import "github.com/DataDog/datadog-agent/pkg/aggregator"

func (c *Check) collectCtxSwitches(sender aggregator.Sender) error {
	// On non-linux systems, do nothing
	return nil
}
