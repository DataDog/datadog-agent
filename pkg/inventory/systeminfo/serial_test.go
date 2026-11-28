// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package systeminfo

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSerialNumberCache(t *testing.T) {
	for _, tt := range []struct {
		name string
		info *SystemInfo
		err  error
	}{
		{name: "error", err: errors.New("hardware unavailable")},
		{name: "no information"},
		{name: "empty serial", info: &SystemInfo{ModelName: "Model"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var c serialNumberCache
			calls := 0
			collect := func() (*SystemInfo, error) {
				calls++
				if calls == 1 {
					return tt.info, tt.err
				}
				return &SystemInfo{SerialNumber: "SERIAL"}, nil
			}
			serial, err := c.get(collect)
			require.ErrorIs(t, err, tt.err)
			require.Empty(t, serial)
			for range 2 {
				serial, err = c.get(collect)
				require.NoError(t, err)
				require.Equal(t, "SERIAL", serial)
			}
			require.Equal(t, 2, calls, "retry missing results and reuse a successful serial")
		})
	}
}

func TestSerialNumberCacheConcurrentCalls(t *testing.T) {
	var c serialNumberCache
	const callers = 20
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			serial, err := c.get(func() (*SystemInfo, error) {
				return &SystemInfo{SerialNumber: "SERIAL"}, nil
			})
			if err != nil || serial != "SERIAL" {
				t.Errorf("unexpected serial %q or error %v", serial, err)
			}
		})
	}
	wg.Wait()
	serial, err := c.get(func() (*SystemInfo, error) {
		t.Error("a cached serial must not trigger another collection")
		return nil, nil
	})
	require.NoError(t, err)
	require.Equal(t, "SERIAL", serial)
}
