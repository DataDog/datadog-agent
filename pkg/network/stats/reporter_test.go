package stats

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStats(t *testing.T) {
	type test struct {
		foo    int64 `stats:""`
		barBaz int32 `stats:"atomic"`
		bar    int
	}

	v := test{foo: 1, barBaz: 2}
	s, err := NewReporter(&v)
	require.NoError(t, err)
	stats := s.Report()
	require.Len(t, stats, 2)
	require.Equal(t, stats["foo"], int64(1))
	require.Equal(t, stats["bar_baz"], int32(2))
}
