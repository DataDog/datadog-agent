package appsec

import (
	"net/http"

	httpapi "github.com/DataDog/datadog-agent/pkg/appsec/api/http"
	"github.com/DataDog/datadog-agent/pkg/appsec/config"
	"github.com/pkg/errors"
)

// ErrAgentDisabled is the error message logged when the AppSec agent is
// disabled by configuration.
var ErrAgentDisabled = errors.New("AppSec agent disabled. Set the " +
	"environment variable `DD_APPSEC_ENABLED=true` or add the entry " +
	"`appsec_config.enabled: true` to your datadog.yaml file")

// New returns the AppSec HTTP handler according to the agent configuration.
func New(agentCfg config.AgentConfig) (http.Handler, error) {
	cfg, err := config.FromAgentConfig(agentCfg)
	if err != nil {
		return nil, errors.Wrap(err, "configuration: ")
	}
	if !cfg.Enabled {
		return nil, ErrAgentDisabled
	}
	return httpapi.NewServeMux(cfg), nil
}
