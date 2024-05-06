// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"expvar"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStats(t *testing.T) {
	myStat := expvar.Int{}

	s, err := NewStats(10)
	require.NoError(t, err)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	require.NotNil(t, ticker)

	go s.Process()
	go s.Update(&myStat)

	deadline := time.After(2 * time.Second)

loop:
	for {
		select {
		case <-ticker.C:
			// send a few events per second
			for i := 0; i < 10; i++ {
				s.StatEvent(int64(i))
			}
		case <-deadline:
			s.Stop()
			break loop
		}
	}

	assert.NotEqual(t, myStat.Value(), 0)
}
