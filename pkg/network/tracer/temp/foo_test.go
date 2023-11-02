package temp

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFoo(t *testing.T) {
	var i int
	require.Eventually(t, func() bool {
		t.Errorf("hello")
		i++
		return i == 5
	}, 3*time.Second, 100*time.Millisecond)
}
