// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package info

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

// atom is a shorthand to create an atomic value
func atom(i int64) atomic.Int64 {
	return *atomic.NewInt64(i)
}

func testExpvarPublish(t *testing.T, publish func() interface{}, expected interface{}) {
	raw := publish()

	// marhsal and unmarshal the result, as expvar would
	marsh, err := json.Marshal(raw)
	require.NoError(t, err)

	var unmarsh interface{}
	err = json.Unmarshal(marsh, &unmarsh)
	require.NoError(t, err)

	require.Equal(t, expected, unmarsh)
}
