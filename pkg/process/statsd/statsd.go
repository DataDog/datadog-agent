// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package statsd

import (
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Client is a global Statsd client. When a client is configured via Configure,
// that becomes the new global Statsd client in the package.
var Client statsd.ClientInterface = &statsd.NoOpClient{}

// Configure creates a statsd client from a dogweb.ini style config file and set it to the global Statsd.
func Configure(host string, port int, create func(string, int, ...statsd.Option) (statsd.ClientInterface, error)) error {
	var err error
	Client, err = create(host, port)
	return err
}
