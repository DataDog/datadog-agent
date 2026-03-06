// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains the logic to create a flare archive.
package common

import (
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v2"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Match .yaml and .yml to ship configuration files in the flare.
var cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)

// GetConfigFiles copies configuration files to the flare archive.
func GetConfigFiles(fb flaretypes.FlareBuilder, confSearchPaths map[string]string) {
	for prefix, filePath := range confSearchPaths {
		fb.CopyDirTo(filePath, filepath.Join("etc", "confd", prefix), func(path string) bool { //nolint:errcheck
			// ignore .example file
			if filepath.Ext(path) == ".example" {
				return false
			}

			firstSuffix := []byte(getFirstSuffix(path))
			ext := []byte(filepath.Ext(path))
			if cnfFileExtRx.Match(firstSuffix) || cnfFileExtRx.Match(ext) {
				return true
			}
			return false
		})
	}

	if pkgconfigsetup.Datadog().ConfigFileUsed() != "" {
		mainConfpath := pkgconfigsetup.Datadog().ConfigFileUsed()
		confDir := filepath.Dir(mainConfpath)

		// zip up the config file that was actually used, if one exists
		fb.CopyFileTo(mainConfpath, filepath.Join("etc", "datadog.yaml")) //nolint:errcheck

		// figure out system-probe file path based on main config path, and use best effort to include
		// system-probe.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "system-probe.yaml"), filepath.Join("etc", "system-probe.yaml")) //nolint:errcheck

		// use best effort to include security-agent.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "security-agent.yaml"), filepath.Join("etc", "security-agent.yaml")) //nolint:errcheck
	}
}

// GetLogFiles copies log files to the flare archive.
func GetLogFiles(fb flaretypes.FlareBuilder, logFileDir string) {
	log.Flush()

	fb.CopyDirToWithoutScrubbing(filepath.Dir(logFileDir), "logs", func(path string) bool { //nolint:errcheck
		if filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log" {
			return true
		}
		return false
	})
}

// GetExpVar copies expvar files to the flare archive.
func GetExpVar(fb flaretypes.FlareBuilder) error {
	variables := make(map[string]interface{})
	expvar.Do(func(kv expvar.KeyValue) {
		variable := make(map[string]interface{})
		json.Unmarshal([]byte(kv.Value.String()), &variable) //nolint:errcheck
		variables[kv.Key] = variable
	})

	// The callback above cannot return an error.
	// In order to properly ensure error checking,
	// it needs to be done in its own loop
	for key, value := range variables {
		yamlValue, err := yaml.Marshal(value)
		if err != nil {
			return err
		}

		err = fb.AddFile(filepath.Join("expvar", key), yamlValue)
		if err != nil {
			return err
		}
	}

	apmDebugPort := pkgconfigsetup.Datadog().GetInt("apm_config.debug.port")
	f := filepath.Join("expvar", "trace-agent")
	resp, err := http.Get(fmt.Sprintf("https://127.0.0.1:%d/debug/vars", apmDebugPort))
	if err != nil {
		return fb.AddFile(f, []byte(fmt.Sprintf("Error retrieving vars: %v", err)))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slurp, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fb.AddFile(f, []byte(fmt.Sprintf("Got response %s from /debug/vars:\n%s", resp.Status, slurp)))
	}
	var all map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return fmt.Errorf("error decoding trace-agent /debug/vars response: %v", err)
	}
	v, err := yaml.Marshal(all)
	if err != nil {
		return err
	}
	return fb.AddFile(f, v)
}

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}
