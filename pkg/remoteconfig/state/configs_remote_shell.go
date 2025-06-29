// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package state

import (
	"encoding/json"
	"fmt"
)

// RemoteShellConfig is a deserialized remote shell configuration file
// along with its associated metadata
type RemoteShellConfig struct {
	Config   RemoteShellData
	Metadata Metadata
}

// RemoteShellData is the content of a remote shell configuration file
type RemoteShellData struct {
	WebsocketURI string            `json:"websocket_uri"`
	PodName      string            `json:"pod_name"`
	Namespace    string            `json:"namespace"`
	Container    string            `json:"container"`
	SessionID    string            `json:"session_id"`
	ExpiresAt    int64             `json:"expires_at"`
	Metadata     map[string]string `json:"metadata"`
}

// ParseConfigRemoteShell parses a remote shell config
func ParseConfigRemoteShell(data []byte, metadata Metadata) (RemoteShellConfig, error) {
	var d RemoteShellData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return RemoteShellConfig{}, fmt.Errorf("unexpected REMOTE_SHELL received through remote-config: %s", err)
	}

	return RemoteShellConfig{
		Config:   d,
		Metadata: metadata,
	}, nil
}

// RemoteShellConfigs returns the currently active REMOTE_SHELL configs
func (r *Repository) RemoteShellConfigs() map[string]RemoteShellConfig {
	typedConfigs := make(map[string]RemoteShellConfig)

	configs := r.getConfigs(ProductRemoteShell)

	for path, conf := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := conf.(RemoteShellConfig)
		if !ok {
			panic("unexpected config stored as RemoteShellConfig")
		}

		typedConfigs[path] = typed
	}

	return typedConfigs
}
