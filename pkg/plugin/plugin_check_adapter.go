package plugin

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pluginApi "github.com/DataDog/datadog-agent/pkg/api/plugin"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

type checkAdapter struct {
	check pluginApi.Check
}

// NewPluginCheckAdapter creates a new adapter that can shuffle information
// between a pluginApi check and a check.Check
func NewPluginCheckAdapter(pluginCheck pluginApi.Check) check.Check {
	return &checkAdapter{
		check: pluginCheck,
	}
}

func (ca *checkAdapter) Run() error {
	sender, err := aggregator.GetSender(ca.ID())
	if err != nil {
		return err
	}

	return ca.check.Run(sender)
}

func (ca *checkAdapter) Stop()          { ca.check.Stop() }
func (ca *checkAdapter) Cancel()        { ca.check.Cancel() }
func (ca *checkAdapter) String() string { return ca.check.String() }
func (ca *checkAdapter) Configure(config, initConfig integration.Data, source string) error {
	return ca.check.Configure(config, initConfig, source)
}
func (ca *checkAdapter) Interval() time.Duration { return ca.check.Interval() }
func (ca *checkAdapter) ID() check.ID            { return check.ID(ca.check.ID()) }
func (ca *checkAdapter) GetWarnings() []error    { return ca.check.GetWarnings() }
func (ca *checkAdapter) GetMetricStats() (map[string]int64, error) {
	return ca.check.GetMetricStats()
}
func (ca *checkAdapter) Version() string          { return ca.check.Version() }
func (ca *checkAdapter) ConfigSource() string     { return ca.check.ConfigSource() }
func (ca *checkAdapter) IsTelemetryEnabled() bool { return ca.check.IsTelemetryEnabled() }
