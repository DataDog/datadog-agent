// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"os"
	"os/user"
	"strings"
)

func NewLocalProfile() (Profile, error) {
	if err := os.MkdirAll(workspaceFolder, 0o700); err != nil {
		return nil, fmt.Errorf("unable to create temporary folder at: %s, err: %w", workspaceFolder, err)
	}

	return localProfile{profile: newProfile("e2elocal", []string{"aws/sandbox"}, nil)}, nil
}

type localProfile struct {
	profile
}

func (p localProfile) RootWorkspacePath() string {
	return workspaceFolder
}

func (p localProfile) NamePrefix() string {
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
