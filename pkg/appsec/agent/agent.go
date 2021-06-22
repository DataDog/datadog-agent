package agent

import (
	"errors"
)

// ErrAgentDisabled is the error message logged when the AppSec agent is
// disabled by configuration.
var ErrAgentDisabled = errors.New("AppSec agent disabled. Set the " +
	"environment variable `DD_APPSEC_ENABLED=true` or add the entry " +
	"`appsec_config.enabled: true` to your datadog.yaml file")
