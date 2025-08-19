// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package datadogrumreceiver

import (
	"time"

	"go.opentelemetry.io/collector/config/confighttp"
)

type Config struct {
	confighttp.ServerConfig `mapstructure:",squash"`
	// ReadTimeout of the http server
	ReadTimeout time.Duration `mapstructure:"read_timeout"`

	// prevent unkeyed literal initialization
	_ struct{}
}
