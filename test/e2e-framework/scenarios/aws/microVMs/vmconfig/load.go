// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package vmconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func LoadConfigFile(filename string) (*Config, error) {
	cfg, err := loadFile(filename)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultValues() *Config {
	return &Config{
		SSHUser: "root",
		Workdir: "/root",
	}
}

func loadFile(filename string) (*Config, error) {

	cfg := defaultValues()
	if filename == "" {
		return nil, fmt.Errorf("loadFile: no config file specified")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("loadFile: failed to read config file: %w", err)
	}
	if err := loadData(data, cfg); err != nil {
		return nil, fmt.Errorf("loadFile: failed to load data: %w", err)
	}

	vmids := make(map[VMSetID]bool)
	for i := range cfg.VMSets {
		set := &cfg.VMSets[i]
		set.ID = VMSetID(fmt.Sprintf("%s-%s", strings.Join(set.Tags, "-"), set.Arch))
		if _, ok := vmids[set.ID]; ok {
			return nil, fmt.Errorf("loadFile: duplicated vmset id: %s", set.ID)
		}

		vmids[set.ID] = true
	}

	return cfg, nil
}

func loadData(data []byte, cfg interface{}) error {
	// Remove comment lines starting with #.
	data = regexp.MustCompile(`(^|\n)\s*#[^\n]*`).ReplaceAll(data, nil)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("loadData: failed to parse config file: %w", err)
	}
	return nil
}
