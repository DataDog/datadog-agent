// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sync"

	procmodel "github.com/DataDog/agent-payload/v5/process"
)

var checkOutputs sync.Map

// StoreCheckOutput stores the output of a check. We use helpers instead of checkOutputs directly to preserve type safety.
func StoreCheckOutput(checkName string, message []procmodel.MessageBody) {
	if message == nil {
		checkOutputs.Store(checkName, []procmodel.MessageBody{})
	} else {
		checkOutputs.Store(checkName, message)
	}
}

// GetCheckOutput retrieves the last output of a check. We use helpers instead of checkOutput directly to preserve type safety.
func GetCheckOutput(checkName string) (value []procmodel.MessageBody, ok bool) {
	if v, ok := checkOutputs.Load(checkName); ok {
		return v.([]procmodel.MessageBody), true
	}
	return nil, false
}
