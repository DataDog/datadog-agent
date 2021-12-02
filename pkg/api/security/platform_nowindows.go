// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows

package security

import (
	"io/ioutil"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// writes auth token(s) to a file that is only readable/writable by the user running the agent
func saveAuthToken(token, tokenPath string) error {
	if err := ioutil.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return err
	}

	return filesystem.ChownDDAgent(tokenPath)
}
