// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.
package memory

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/require"
)

func TestCollectMemory(t *testing.T) {
	memInfo := CollectInfo()

	_, err := memInfo.TotalBytes.Value()
	if err != nil {
		require.ErrorIs(t, err, utils.ErrNotCollectable)
	}

	_, err = memInfo.SwapTotalKb.Value()
	if err != nil {
		require.ErrorIs(t, err, utils.ErrNotCollectable)
	}
}

func TestMemoryAsJSON(t *testing.T) {
	memInfo := CollectInfo()
	marshallable, _, err := memInfo.AsJSON()
	require.NoError(t, err)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	// Any change to this datastructure should be notified to the backend
	// team to ensure compatibility.
	type Memory struct {
		Total     string `json:"total"`
		SwapTotal string `json:"swap_total"`
	}

	decoder := json.NewDecoder(bytes.NewReader(marshalled))
	// do not ignore unknown fields
	decoder.DisallowUnknownFields()

	var decodedMem Memory
	err = decoder.Decode(&decodedMem)
	require.NoError(t, err)

	// check that we read the full json
	require.False(t, decoder.More())

	if _, err = memInfo.TotalBytes.Value(); err == nil {
		// the total field is just a number of bytes, without explicit units
		require.Regexp(t, `^\d+$`, decodedMem.Total)
	}
	if _, err = memInfo.SwapTotalKb.Value(); err == nil {
		// the swap total field is a number with unit kb
		require.Regexp(t, `^\d+kb$`, decodedMem.SwapTotal)
	}
}
