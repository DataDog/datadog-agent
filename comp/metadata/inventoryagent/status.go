// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagent

import (
	"github.com/DataDog/datadog-agent/comp/core/status"
)

// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
func (ia *inventoryagent) statusProvider() status.Provider {
	return status.NewProvider(func(stats map[string]interface{}) {
		data := map[string]interface{}{}
		for k, v := range ia.data {
			data[k] = v
		}
		stats["agent_metadata"] = data
	})
}
