package backoff

import (
	"math"
	"math/rand"
	"time"
)

// ExponentialConfig holds the parameters used by the ExponentialTimer.
type ExponentialConfig struct {
	MaxDuration time.Duration
	GrowthBase  int
	Base        time.Duration
}

// DefaultExponentialConfig creates an ExponentialConfig with default values.
func DefaultExponentialConfig() ExponentialConfig {
	return ExponentialConfig{
		MaxDuration: 120 * time.Second,
		GrowthBase:  2,
		Base:        200 * time.Millisecond,
	}
}

// DefaultExponentialDelayProvider creates a new instance of an ExponentialDelayProvider using the default config.
func DefaultExponentialDelayProvider() DelayProvider {
	return ExponentialDelayProvider(DefaultExponentialConfig())
}

// ExponentialDelayProvider creates a new instance of an ExponentialDelayProvider using the provided config.
func ExponentialDelayProvider(conf ExponentialConfig) DelayProvider {
	return exponentialDelayProviderCustomRandom(conf, rand.New(rand.NewSource(time.Now().UnixNano())))
}

// exponentialDelayProviderCustomRandom creates a new instance of ExponentialDelayProvider using the provided config
// and random number generator.
func exponentialDelayProviderCustomRandom(conf ExponentialConfig, rand *rand.Rand) DelayProvider {
	return func(numRetries int, _ error) time.Duration {
		pow := math.Pow(float64(conf.GrowthBase), float64(numRetries))

		// Correctly handle overflowing pow
		if pow < 0 || pow > math.MaxInt64 {
			pow = math.MaxInt64
		}

		mul := int64(pow) * int64(conf.Base)

		// Correctly handle overflowing mul
		if pow != 0 && mul/int64(pow) != int64(conf.Base) {
			mul = math.MaxInt64
		}

		newExpDuration := time.Duration(mul)

		if newExpDuration > conf.MaxDuration {
			newExpDuration = conf.MaxDuration
		}

		return time.Duration(rand.Int63n(int64(newExpDuration)))
	}
}

// ExponentialTimer performs an exponential backoff following the FullJitter implementation described in
// https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
type ExponentialTimer struct {
	CustomTimer
}

// NewExponentialTimer creates an exponential backoff timer using the default configuration.
func NewExponentialTimer() *ExponentialTimer {
	return NewCustomExponentialTimer(DefaultExponentialConfig())
}

// NewCustomExponentialTimer creates an exponential backoff timer using the provided configuration.
func NewCustomExponentialTimer(conf ExponentialConfig) *ExponentialTimer {
	return &ExponentialTimer{
		CustomTimer: *NewCustomTimer(ExponentialDelayProvider(conf)),
	}
}
