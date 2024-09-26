package host

import (
	"github.com/benbjohnson/clock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dependencies struct {
	fx.In

	Config config.Component
}

type HostTagManager struct {
	clock        clock.Clock
	timeoutTimer *clock.Timer
	config       config.Component
}

func GetFxOptions() fx.Option {
	return fx.Provide(NewHostTagManager)
}

// NewHostTagManager creates a new HostTagManager instance with the specified duration and clock.
func NewHostTagManager(deps dependencies) *HostTagManager {
	return &HostTagManager{
		clock:  clock.New(),
		config: deps.Config,
	}
}

// Start initializes the expiration timer.
func (htm *HostTagManager) Start() {
	duration := htm.config.GetDuration("expected_tags_duration")
	if duration > 0 {
		log.Debugf("Adding host tags to metrics for %v", duration)
		htm.timeoutTimer = htm.clock.Timer(duration)
	}
}

// ShouldExpire checks if the timer has expired.
func (htm *HostTagManager) ShouldExpire() bool {
	if htm.timeoutTimer == nil {
		return false
	}

	select {
	case <-htm.timeoutTimer.C:
		htm.timeoutTimer = nil
		return true
	default:
		return false
	}
}
