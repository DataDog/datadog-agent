// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusinterface describes status methods required in logs agent modules
package statusinterface

// Status is the type for status methods
type Status interface {
	// AddGlobalWarning keeps track of a warning message to display on the status.
	AddGlobalWarning(key string, warning string)

	// RemoveGlobalWarning loses track of a warning message
	// that does not need to be displayed on the status anymore.
	RemoveGlobalWarning(key string)
}
