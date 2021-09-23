package time

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSince(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		start := Now()
		Sleep(10 * Millisecond)
		require.GreaterOrEqual(t, Since(start), 5*Millisecond)
	})
}

func TestUntil(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		start := Now()
		later := start.Add(Minute)
		require.LessOrEqual(t, Until(later), Minute)
	})
}
