// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
)

func NewLocalProfile() (Profile, error) {
	if err := os.MkdirAll(workspaceFolder, 0o700); err != nil {
		return nil, fmt.Errorf("unable to create temporary folder at: %s, err: %w", workspaceFolder, err)
	}
	envValueStore := parameters.NewEnvValueStore(EnvPrefix)

	configPath, err := getConfigFilePath()
	if err != nil {
		return nil, err
	}

	var store parameters.Store
	if configPath != "" {
		configFileValueStore, err := parameters.NewConfigFileValueStore(configPath)
		if err != nil {
			return nil, fmt.Errorf("error when reading the config file %v: %v", configPath, err)
		}
		store = parameters.NewCascadingStore(envValueStore, configFileValueScore)
	} else {
		store = parameters.NewCascadingStore(envValueStore)
	}
	return localProfile{baseProfile: newProfile("e2elocal", []string{"aws/sandbox"}, store, nil)}, nil
}

func getConfigFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to get the home dir")
	}
	configPath := path.Join(homeDir, ".test_infra_config.yaml")

	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return configPath, nil
}

type localProfile struct {
	baseProfile
}

func (p localProfile) RootWorkspacePath() string {
	return workspaceFolder
}

func (p localProfile) NamePrefix() string {
	// Stack names may only contain alphanumeric characters, hyphens, underscores, or periods.
	// As NamePrefix is used as stack name, we sanitize the user name.
	var username string
	user, err := user.Current()
	if err == nil {
		username = user.Username
	}

	if username == "" || username == "root" {
		username = "nouser"
	}

	parts := strings.Split(username, ".")
	if numParts := len(parts); numParts > 1 {
		var usernameBuilder strings.Builder
		for _, part := range parts[0 : numParts-1] {
			usernameBuilder.WriteByte(part[0])
		}
		usernameBuilder.WriteString(parts[numParts-1])
		username = usernameBuilder.String()
	}

	username = strings.ToLower(username)
	username = strings.ReplaceAll(username, " ", "-")

	return username
}

func (p localProfile) AllowDevMode() bool {
	return true
}
