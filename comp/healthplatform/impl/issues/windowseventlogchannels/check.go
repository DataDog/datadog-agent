// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowseventlogchannels

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// win32EventLogConfig represents the structure of a win32_event_log configuration file.
type win32EventLogConfig struct {
	Instances []struct {
		ChannelPath string `yaml:"channel_path"`
	} `yaml:"instances"`
}

// Check scans win32_event_log configuration files and verifies that all configured
// channel_path values correspond to channels that exist on the host.
func Check(cfg config.Component) (*healthplatform.IssueReport, error) {
	confdPath := cfg.GetString("confd_path")
	if confdPath == "" {
		return nil, nil
	}

	configDir := filepath.Join(confdPath, "win32_event_log.d")
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Integration not configured at all — nothing to check
			return nil, nil
		}
		return nil, err
	}

	// Collect all configured channel names across config files, tracking the
	// first config file that references each channel for context reporting.
	type channelRef struct {
		channel    string
		configFile string
	}
	var channelRefs []channelRef

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		fullPath := filepath.Join(configDir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			log.Warnf("windowseventlogchannels: could not read %s: %v", fullPath, err)
			continue
		}

		var conf win32EventLogConfig
		if err := yaml.Unmarshal(data, &conf); err != nil {
			log.Warnf("windowseventlogchannels: could not parse %s: %v", fullPath, err)
			continue
		}

		for _, inst := range conf.Instances {
			ch := strings.TrimSpace(inst.ChannelPath)
			if ch == "" {
				continue
			}
			channelRefs = append(channelRefs, channelRef{
				channel:    ch,
				configFile: filepath.Join("win32_event_log.d", name),
			})
		}
	}

	if len(channelRefs) == 0 {
		return nil, nil
	}

	// Verify each channel exists using wevtutil gl <channel>.
	var invalidChannels []string
	var firstInvalidConfigFile string
	for _, ref := range channelRefs {
		if !channelExists(ref.channel) {
			invalidChannels = append(invalidChannels, ref.channel)
			if firstInvalidConfigFile == "" {
				firstInvalidConfigFile = ref.configFile
			}
		}
	}

	if len(invalidChannels) == 0 {
		return nil, nil
	}

	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"invalidChannels": strings.Join(invalidChannels, ","),
			"configFile":      firstInvalidConfigFile,
		},
		Tags: []string{"windows-event-log", "configuration"},
	}, nil
}

// channelExists returns true if the given Windows Event Log channel exists on the host.
// It uses wevtutil gl <channel> which exits with a non-zero code when the channel is unknown.
func channelExists(channel string) bool {
	cmd := exec.Command("wevtutil", "gl", channel)
	err := cmd.Run()
	return err == nil
}
