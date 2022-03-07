package dockerork8s

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/stretchr/testify/require"
)

func fastRetrier() *retry.Retrier {
	retr := &retry.Retrier{}
	retr.SetupRetrier(&retry.Config{
		Strategy:          retry.Backoff,
		InitialRetryDelay: 1 * time.Millisecond,
		MaxRetryDelay:     1 * time.Millisecond,
	})
	return retr
}

// immediately succeed
func immediatelyChoose() (bool, *retry.Retrier) {
	return true, nil
}

// never succeed
func waitForever() (bool, *retry.Retrier) {
	return false, fastRetrier()
}

// succeed on the second try
func secondTry() func() (bool, *retry.Retrier) {
	calls := 0
	return func() (bool, *retry.Retrier) {
		if calls == 0 {
			calls++
			return false, fastRetrier()
		}
		return true, nil
	}
}

func TestNoneAvailableOnStop(t *testing.T) {
	dok := New(Docker)
	dok.isAvailable[Docker] = waitForever
	dok.isAvailable[K8s] = waitForever
	dok.Start()
	ch := dok.Subscribe()
	dok.Stop()
	require.Equal(t, None, <-ch)
}

func TestOneAvailableImmediately(t *testing.T) {
	test := func(choice Choice) func(*testing.T) {
		return func(t *testing.T) {
			dok := New(Docker)
			dok.isAvailable[choice] = immediatelyChoose
			dok.isAvailable[choice.opposite()] = waitForever
			dok.Start()
			ch := dok.Subscribe()
			require.Equal(t, choice, <-ch)
			dok.Stop()
		}
	}
	t.Run("Docker", test(Docker))
	t.Run("K8s", test(K8s))
}

func TestOneAvailableSecondTry(t *testing.T) {
	test := func(choice Choice) func(*testing.T) {
		return func(t *testing.T) {
			dok := New(Docker)
			dok.isAvailable[choice] = secondTry()
			dok.isAvailable[choice.opposite()] = waitForever
			dok.Start()
			ch := dok.Subscribe()
			require.Equal(t, choice, <-ch)
			dok.Stop()
		}
	}
	t.Run("Docker", test(Docker))
	t.Run("K8s", test(K8s))
}

func TestImmediateChoiceTakesPreferred(t *testing.T) {
	test := func(choice Choice) func(*testing.T) {
		return func(t *testing.T) {
			dok := New(choice)
			dok.isAvailable[choice] = immediatelyChoose
			dok.isAvailable[choice.opposite()] = immediatelyChoose
			dok.Start()
			ch := dok.Subscribe()
			require.Equal(t, choice, <-ch)
			dok.Stop()
		}
	}
	t.Run("Docker", test(Docker))
	t.Run("K8s", test(K8s))
}
