// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winregistry defines the winregistry check
package winregistry

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util"
	"io/fs"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/registry"
	"gopkg.in/yaml.v2"
)

const (
	checkName   = "windows_registry" // This appears in the Agent Manager and Agent status
	checkPrefix = "winregistry"      // This is the prefix used for all metrics emitted by this check
)

type registryValueCfg struct {
	Name         string                 `yaml:"name"` // The metric name of the registry value
	DefaultValue util.Optional[float64] `yaml:"default_value"`
}

type registryKeyCfg struct {
	Name           string                      `yaml:"name"`            // The metric name of the registry key
	RegistryValues map[string]registryValueCfg `yaml:"registry_values"` // The map key is the registry value name
}

type checkCfg struct {
	RegistryKeys map[string]registryKeyCfg `yaml:"registry_keys"`
}

// registryKey is the in-memory representation of the key to monitor
// it's different from the registryKeyCfg because we need to split the hive
// from the keypath. It's easier to do this once during the Configure phase,
// than every time the check runs.
type registryKey struct {
	name            string
	hive            registry.Key
	keyPath         string
	originalKeyPath string // keep the original keypath around, for logging errors
	registryValues  map[string]registryValueCfg
}

// WindowsRegistryCheck contains the field for the WindowsRegistryCheck
type WindowsRegistryCheck struct {
	core.CheckBase
	metrics.Gauge
	sender       sender.Sender
	registryKeys []registryKey
}

// Configure reads the config and setups the check
func (c *WindowsRegistryCheck) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	c.BuildID(integrationConfigDigest, data, initConfig)
	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	var conf checkCfg
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return err
	}

	hiveMap := map[string]registry.Key{
		"HKLM":               registry.LOCAL_MACHINE,
		"HKEY_LOCAL_MACHINE": registry.LOCAL_MACHINE,
		"HKU":                registry.USERS,
		"HKEY_USERS":         registry.USERS,
		"HKEY_CLASSES_ROOT":  registry.CLASSES_ROOT,
		"HKCR":               registry.CLASSES_ROOT,
	}

	for regKey, regKeyConfig := range conf.RegistryKeys {
		splitKeypath := strings.SplitN(regKey, "\\", 2)
		if len(splitKeypath) != 2 {
			return log.Errorf("the key %s is too short to be a valid key", regKey)
		}

		if len(regKeyConfig.Name) == 0 {
			log.Warnf("the key %s does not have a metric name, skipping", regKey)
			continue
		}

		if hive, found := hiveMap[splitKeypath[0]]; found {
			regValues := make(map[string]registryValueCfg)
			for valueName, regValueCfg := range regKeyConfig.RegistryValues {
				if len(regValueCfg.Name) == 0 {
					log.Warnf("the subkey %s of %s does not have a metric name, skipping", valueName, regKey)
				} else {
					regValues[valueName] = regValueCfg
				}
			}
			c.registryKeys = append(c.registryKeys, registryKey{
				name:            regKeyConfig.Name,
				hive:            hive,
				originalKeyPath: regKey,
				keyPath:         splitKeypath[1],
				registryValues:  regValues,
			})
		} else {
			return log.Errorf("unknown hive %s", splitKeypath[0])
		}
	}

	c.sender, err = c.GetSender()
	if err != nil {
		log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
		return err
	}
	c.sender.FinalizeCheckServiceTag()

	return nil
}

// Run runs the check
func (c *WindowsRegistryCheck) Run() error {
	for _, regKeyCfg := range c.registryKeys {
		regKey, err := registry.OpenKey(regKeyCfg.hive, regKeyCfg.keyPath, registry.QUERY_VALUE)
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				// Treat access denied as errors
				log.Errorf("access denied while accessing key %s: %s", regKeyCfg.originalKeyPath, err)
			} else if errors.Is(err, registry.ErrNotExist) {
				log.Warnf("key %s was not found: %s", regKeyCfg.originalKeyPath, err)
				// Process registryValues too so that we can emit missing values for each registryValues
				processRegistryKeyMetrics(c.sender, regKey, regKeyCfg)
			}
		} else {
			// if err == nil the key was opened, so we need to close it after we are done.
			processRegistryKeyMetrics(c.sender, regKey, regKeyCfg)
			regKey.Close()
		}
	}
	c.sender.Commit()
	return nil
}

func processRegistryKeyMetrics(sender sender.Sender, regKey registry.Key, regKeyCfg registryKey) {
	for valueName, regValueCfg := range regKeyCfg.registryValues {
		var err error
		var valueType uint32
		// regKey == 0 means parent key didn't exist, but we want to emit missing metric for each of its values
		if regKey == 0 {
			err = registry.ErrNotExist
		} else {
			_, valueType, err = regKey.GetValue(valueName, nil)
		}
		gaugeName := fmt.Sprintf("%s.%s.%s", checkPrefix, regKeyCfg.name, regValueCfg.Name)
		if errors.Is(err, registry.ErrNotExist) {
			log.Warnf("value %s of key %s was not found: %s", valueName, regKeyCfg.name, err)
			trySendDefaultValue(sender, regValueCfg, gaugeName)
		} else if errors.Is(err, fs.ErrPermission) {
			log.Errorf("access denied while accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
			trySendDefaultValue(sender, regValueCfg, gaugeName)
		} else if errors.Is(err, registry.ErrShortBuffer) || err == nil {
			switch valueType {
			case registry.DWORD:
				fallthrough
			case registry.QWORD:
				val, _, err := regKey.GetIntegerValue(valueName)
				if err != nil {
					log.Errorf("error accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
					trySendDefaultValue(sender, regValueCfg, gaugeName)
					continue
				}
				sender.Gauge(gaugeName, float64(val), "", nil)
			case registry.SZ:
				fallthrough
			case registry.EXPAND_SZ:
				val, _, err := regKey.GetStringValue(valueName)
				if valueType == registry.EXPAND_SZ {
					val, err = registry.ExpandString(val)
				}
				if err != nil {
					log.Errorf("error accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
					trySendDefaultValue(sender, regValueCfg, gaugeName)
					continue
				}
				// First try to parse the value into a float64
				if parsedVal, err := strconv.ParseFloat(val, 64); err == nil {
					sender.Gauge(gaugeName, parsedVal, "", nil)
				} else {
					log.Warnf("value %s of key %s cannot be parsed", valueName, regKeyCfg.originalKeyPath)
					trySendDefaultValue(sender, regValueCfg, gaugeName)
				}
			default:
				log.Warnf("unsupported data type of value %s for key %s: %d", valueName, regKeyCfg.originalKeyPath, valueType)
				trySendDefaultValue(sender, regValueCfg, gaugeName)
			}
		}
	}
}

func trySendDefaultValue(sender sender.Sender, regValueCfg registryValueCfg, gaugeName string) {
	if defaultVal, exists := regValueCfg.DefaultValue.Get(); exists {
		sender.Gauge(gaugeName, defaultVal, "", nil)
	}
}

func windowsRegistryCheckFactory() check.Check {
	return &WindowsRegistryCheck{
		CheckBase: core.NewCheckBase(checkName),
	}
}

func init() {
	core.RegisterCheck(checkName, windowsRegistryCheckFactory)
}
