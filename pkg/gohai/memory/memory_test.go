// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.
package memory

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/assert"
)

func TestCollectMemory(t *testing.T) {
	memInfo := CollectInfo()

	errorGetters := map[string]error{
		"TotalBytes":  memInfo.TotalBytes.Error(),
		"SwapTotalKb": memInfo.SwapTotalKb.Error(),
	}

	for fieldname, err := range errorGetters {
		if err != nil {
			assert.ErrorIsf(t, err, utils.ErrNotCollectable, "memory: field %s could not be collected", fieldname)
		}
	}
}

func TestMemoryAsJSON(t *testing.T) {
	memInfo := CollectInfo()

	// Any change to this datastructure should be notified to the backend
	// team to ensure compatibility.
	type Memory struct {
		Total     string `json:"total"`
		SwapTotal string `json:"swap_total"`
	}

	var decodedMem Memory
	utils.RequireMarshallJSON(t, memInfo, &decodedMem)

	utils.AssertDecodedValue(t, decodedMem.Total, &memInfo.TotalBytes, "")
	utils.AssertDecodedValue(t, decodedMem.SwapTotal, &memInfo.SwapTotalKb, "kB")
}
