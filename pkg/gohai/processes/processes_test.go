// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin

package processes

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectInfo(t *testing.T) {
	processes, err := CollectInfo()
	require.NoError(t, err)

	require.NotEmpty(t, processes)
	for _, process := range processes {
		assert.NotEmpty(t, process.Name)

		assert.NotEmpty(t, process.Usernames)
		assert.IsIncreasing(t, process.Usernames)

		assert.GreaterOrEqual(t, process.PctCPU, 0)
		assert.LessOrEqual(t, process.PctCPU, 100)

		assert.GreaterOrEqual(t, process.PctMem, float64(0))
		assert.LessOrEqual(t, process.PctMem, float64(100))

		assert.GreaterOrEqual(t, process.VMS, uint64(0))
		assert.GreaterOrEqual(t, process.RSS, uint64(0))

		assert.NotEmpty(t, process.Pids)
	}
}

func TestAsJSON(t *testing.T) {
	info, err := CollectInfo()
	require.NoError(t, err)

	result, _, err := info.AsJSON()
	require.NoError(t, err)

	marshalled, err := json.Marshal(result)
	require.NoError(t, err)

	// we can't easily Unmarshal the object in a struct since it uses arrays with different types
	// so we have to use interface{} and check

	var payload [2]interface{}
	err = json.Unmarshal(marshalled, &payload)
	require.NoError(t, err)

	// the default type used when decoding integers is float64 so we have to work around that
	timestamp, ok := payload[0].(float64)
	require.True(t, ok)
	require.Greater(t, timestamp, float64(0))
	require.LessOrEqual(t, timestamp, float64(time.Now().Unix()))

	processes, ok := payload[1].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, processes)

	for _, process := range processes {
		process, ok := process.([]interface{})
		require.True(t, ok)
		require.Equal(t, len(process), 7)

		usernames, ok := process[0].(string)
		require.True(t, ok)
		assert.NotEmpty(t, usernames)

		pctCPU, ok := process[1].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, pctCPU, 0.)
		assert.LessOrEqual(t, pctCPU, 100.)

		pctMem, ok := process[2].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, pctMem, float64(0))
		assert.LessOrEqual(t, pctMem, float64(100))

		vms, ok := process[3].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, vms, 0.)

		rss, ok := process[4].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, rss, 0.)

		name, ok := process[5].(string)
		require.True(t, ok)
		assert.NotEmpty(t, name)

		nbPids, ok := process[6].(float64)
		require.True(t, ok)
		assert.Greater(t, nbPids, 0.)
	}
}
