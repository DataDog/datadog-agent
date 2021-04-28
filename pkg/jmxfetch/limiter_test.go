package jmxfetch

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCanRestart(t *testing.T) {
	type check struct {
		now    time.Duration
		result bool
	}
	tests := []struct {
		maxRestarts int
		interval    float64
		checks      []check
	}{
		{-1, 10.0, []check{{0 * time.Second, false}}},
		{0, 10.0, []check{{0 * time.Second, false}}},
		{1, 10.0, []check{{0 * time.Second, false}}},
		{2, 10.0, []check{{0 * time.Second, true}, {1 * time.Second, false}}},
		{3, 10.0, []check{{0 * time.Second, true}, {1 * time.Second, true},
			{13 * time.Second, true}, {14 * time.Second, true}, {15 * time.Second, false}}},
	}

	tref := time.Now()
	for tidx, tt := range tests {
		r := newRestartLimiter(tt.maxRestarts, tt.interval)
		for cidx, check := range tt.checks {
			name := fmt.Sprintf("%d/%d", tidx, cidx)
			t.Run(name, func(t *testing.T) {
				require.Equal(t, check.result, r.canRestart(tref.Add(check.now)))
			})
		}
	}
}
