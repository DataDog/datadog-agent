// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package hostname

import (
	"errors"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func init() {
	RegisterHostnameProvider(Lowest, "OS", osHostname)
}

func osHostname(name string) (string, error) {
	if config.IsContainerized() {
		return "", errors.New("can't use OS hostname in a container")
	}
	return os.Hostname()
}
