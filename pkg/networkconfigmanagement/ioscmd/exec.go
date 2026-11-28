// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ioscmd

// PingCommand is `ping <dest>` or `traceroute <dest>`.
type PingCommand struct {
	// Type is "ping" or "traceroute".
	Type string `json:"type"`
	Dest string `json:"dest"`
}

func (PingCommand) isConfig() bool { return false }

func (c PingCommand) render() ([]string, error) {
	if err := checkOneOf("type", c.Type, "ping", "traceroute"); err != nil {
		return nil, err
	}
	if err := checkIPv4("dest", c.Dest); err != nil {
		return nil, err
	}
	return []string{c.Type + " " + c.Dest}, nil
}

// CopyRunningConfigCommand is `copy running-config startup-config`, which
// saves the running config.
type CopyRunningConfigCommand struct {
	// Type is always "copy running-config startup-config".
	Type string `json:"type"`
}

func (CopyRunningConfigCommand) isConfig() bool { return false }

// render follows the command with a blank line, which accepts the default
// answer to the "Destination filename [startup-config]?" prompt.
func (CopyRunningConfigCommand) render() ([]string, error) {
	return []string{"copy running-config startup-config", ""}, nil
}
