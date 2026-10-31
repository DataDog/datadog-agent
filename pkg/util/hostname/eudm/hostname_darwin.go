// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import "github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"

var localHostName = systeminfo.LocalHostName

// deviceName preserves Apple's normalization and any Bonjour conflict suffix.
func deviceName() (string, error) {
	return localHostName()
}
