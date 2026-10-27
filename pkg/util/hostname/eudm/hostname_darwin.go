// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import "github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"

// deviceName uses the configured Computer Name, which network changes do not affect.
func deviceName(info *systeminfo.SystemInfo) (string, error) {
	return info.ComputerName, nil
}
