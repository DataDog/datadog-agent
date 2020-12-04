// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build !linux

package system

import "github.com/DataDog/datadog-agent/pkg/aggregator"

func (c *CPUCheck) collectCtxSwitches(sender aggregator.Sender) error {
	// On non-linux systems, do nothing
	return nil
}
