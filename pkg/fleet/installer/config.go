// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigActionType is the type of action to perform on the config.
type ConfigActionType string

const (
	// ConfigActionTypeWrite sets the value of the config.
	ConfigActionTypeWrite ConfigActionType = "write"
	// ConfigActionTypeMerge merges the current config with the override config.
	ConfigActionTypeMerge ConfigActionType = "merge"
	// ConfigActionTypeDelete deletes the current config.
	ConfigActionTypeDelete ConfigActionType = "delete"
)

// ConfigAction is the action to perform on a config.
type ConfigAction struct {
	ActionType    ConfigActionType `json:"action_type"`
	Path          string           `json:"path"`
	Value         any              `json:"value"`
	IgnoredFields []string         `json:"ignored_fields"`
}

func (a *ConfigAction) apply(root *os.Root) error {
	if !configNameAllowed(a.Path) {
		return fmt.Errorf("modifying config file %s is not allowed", a.Path)
	}
	switch a.ActionType {
	case ConfigActionTypeWrite:
		file, err := root.OpenFile(a.Path, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer file.Close()
		rawValue, err := yaml.Marshal(a.Value)
		if err != nil {
			return err
		}
		_, err = file.Write(rawValue)
		return err
	case ConfigActionTypeMerge:
		file, err := root.OpenFile(a.Path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return err
		}
		defer file.Close()
		currentRawValue, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		var currentValue any
		err = yaml.Unmarshal(currentRawValue, &currentValue)
		if err != nil {
			return err
		}
		mergedValue, err := merge(currentValue, a.Value)
		if err != nil {
			return err
		}
		rawMergedValue, err := yaml.Marshal(mergedValue)
		if err != nil {
			return err
		}
		err = file.Truncate(0)
		if err != nil {
			return err
		}
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		_, err = file.Write(rawMergedValue)
		return err
	case ConfigActionTypeDelete:
		return root.Remove(a.Path)
	}
	return nil
}

var (
	allowedConfigFiles = []string{
		"/datadog.yaml",
		"/otel-config.yaml",
		"/security-agent.yaml",
		"/system-probe.yaml",
		"/application_monitoring.yaml",
		"/conf.d/*.yaml",
		"/conf.d/*.d/*.yaml",
	}
)

func configNameAllowed(file string) bool {
	for _, allowedFile := range allowedConfigFiles {
		match, err := filepath.Match(allowedFile, file)
		if err != nil {
			return false
		}
		if match {
			return true
		}
	}
	return false
}

// writeConfigSymlinks writes `.override` symlinks to help surface configurations to the user
func writeConfigSymlinks(userDir string, fleetDir string) error {
	userFiles, err := os.ReadDir(userDir)
	if err != nil {
		return fmt.Errorf("could not list user config files: %w", err)
	}
	for _, userFile := range userFiles {
		if userFile.Type()&os.ModeSymlink != 0 && strings.HasSuffix(userFile.Name(), ".override") {
			err = os.Remove(filepath.Join(userDir, userFile.Name()))
			if err != nil {
				return fmt.Errorf("could not remove existing symlink: %w", err)
			}
		}
	}
	var files []string
	fleetFiles, err := os.ReadDir(fleetDir)
	if err != nil {
		return fmt.Errorf("could not list fleet config files: %w", err)
	}
	for _, fleetFile := range fleetFiles {
		files = append(files, fleetFile.Name())
	}
	for _, file := range files {
		overrideFile := file + ".override"
		err = os.Symlink(filepath.Join(fleetDir, file), filepath.Join(userDir, overrideFile))
		if err != nil {
			return fmt.Errorf("could not create symlink: %w", err)
		}
	}
	return nil
}

// merge merges the current object with the override object.
//
// The values are merged as follows:
// - Scalars: the override value is used
// - Lists: the override list is used
// - Maps: the override map is recursively merged into the base map
func merge(base any, override any) (any, error) {
	if base == nil {
		return override, nil
	}
	if override == nil {
		// this allows to delete a value with nil
		return nil, nil
	}
	if isScalar(base) && isScalar(override) {
		return override, nil
	}
	if isList(base) && isList(override) {
		return override, nil
	}
	if isMap(base) && isMap(override) {
		return mergeMap(base.(map[string]any), override.(map[string]any))
	}
	return nil, fmt.Errorf("could not merge %T with %T", base, override)
}

func mergeMap(base, override map[string]any) (map[string]any, error) {
	merged := make(map[string]any)
	maps.Copy(merged, base)
	for k := range override {
		v, err := merge(base[k], override[k])
		if err != nil {
			return nil, fmt.Errorf("could not merge key %v: %w", k, err)
		}
		merged[k] = v
	}
	return merged, nil
}

func isList(i any) bool {
	_, ok := i.([]any)
	return ok
}

func isMap(i any) bool {
	_, ok := i.(map[string]any)
	return ok
}

func isScalar(i any) bool {
	return !isList(i) && !isMap(i)
}
