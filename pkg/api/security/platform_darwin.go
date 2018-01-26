// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package security

import (
	"io/ioutil"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(token, tokenPath string) error {
	confFile, err := os.Stat(config.Datadog.ConfigFileUsed())
	if err != nil {
		return err
	}
	permissions := confFile.Mode()

	return ioutil.WriteFile(tokenPath, []byte(token), permissions)
}
