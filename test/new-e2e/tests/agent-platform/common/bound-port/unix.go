// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package boundport

import (
	"errors"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

func boundPortsUnix(host *components.RemoteHost) ([]BoundPort, error) {
	if _, err := host.Execute("command -v netstat"); err == nil {
		out, err := host.Execute("sudo netstat -plunt")
		if err != nil {
			return nil, err
		}
		return FromNetstat(out)
	}

	if _, err := host.Execute("command -v ss"); err == nil {
		out, err := host.Execute("sudo ss -plunt")
		if err != nil {
			return nil, err
		}
		return FromSs(out)
	}

	return nil, errors.New("no ss found")
}
