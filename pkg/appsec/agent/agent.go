package agent

import (
	"errors"
)

var ErrAgentDisabled = errors.New("AppSec agent disabled. Set the " +
	"environment variable `DD_APPSEC_ENABLED=true` or add the entry " +
	"`appsec_config.enabled: true` to your datadog.yaml file")
