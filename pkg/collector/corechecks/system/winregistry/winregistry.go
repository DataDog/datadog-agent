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
	"io/fs"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util"

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

type subkeyCfg struct {
	Name         string                 `yaml:"name"` // The metric name of the registry value
	DefaultValue util.Optional[float64] `yaml:"default_value"`
}

type registryKeyCfg struct {
	Name    string               `yaml:"name"`    // The metric name of the registry key
	Subkeys map[string]subkeyCfg `yaml:"subkeys"` // The map key is the subkey name
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
	subkeys         map[string]subkeyCfg
}

// WindowsRegistryCheck contains the field for the WindowsRegistryCheck
type WindowsRegistryCheck struct {
	core.CheckBase
	metrics.Gauge
	registryKeys []registryKey
}

// Configure reads the config and setups the check
func (c *WindowsRegistryCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
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
		splitKeypath := strings.Split(regKey, "\\")
		if len(splitKeypath) < 2 {
			return log.Errorf("the key %s is too short to be a valid key", regKey)
		}

		if len(regKeyConfig.Name) == 0 {
			log.Warnf("the key %s does not have a metric name, skipping", regKey)
			continue
		}

		if hive, found := hiveMap[splitKeypath[0]]; found {
			subkeys := make(map[string]subkeyCfg)
			for valueName, regKeyCfg := range regKeyConfig.Subkeys {
				if len(regKeyCfg.Name) == 0 {
					log.Warnf("the subkey %s of %s does not have a metric name, skipping", valueName, regKey)
				} else {
					subkeys[valueName] = regKeyCfg
				}
			}
			c.registryKeys = append(c.registryKeys, registryKey{
				name:            regKeyConfig.Name,
				hive:            hive,
				originalKeyPath: regKey,
				keyPath:         strings.Join(splitKeypath[1:], "\\"),
				subkeys:         subkeys,
			})
		} else {
			return log.Errorf("unknown hive %s", splitKeypath[0])
		}
	}

	return nil
}

// Run runs the check
func (c *WindowsRegistryCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	for _, regKeyCfg := range c.registryKeys {
		regKey, err := registry.OpenKey(regKeyCfg.hive, regKeyCfg.keyPath, registry.QUERY_VALUE)
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				// Treat access denied as errors
				log.Errorf("access denied while accessing key %s: %s", regKeyCfg.originalKeyPath, err)
			} else if errors.Is(err, registry.ErrNotExist) {
				log.Warnf("key %s was not found: %s", regKeyCfg.originalKeyPath, err)
			}
		} else {
			// if err == nil the key was opened, so we need to close it after we are done.
			processRegistryKeyMetrics(sender, regKey, regKeyCfg)
			regKey.Close()
		}
	}
	sender.Commit()
	return nil
}

func processRegistryKeyMetrics(sender sender.Sender, regKey registry.Key, regKeyCfg registryKey) {
	for valueName, subkey := range regKeyCfg.subkeys {
		_, valueType, err := regKey.GetValue(valueName, nil)
		gaugeName := fmt.Sprintf("%s.%s.%s", checkPrefix, regKeyCfg.name, subkey.Name)
		if errors.Is(err, registry.ErrNotExist) {
			log.Warnf("value %s of key %s was not found: %s", valueName, regKeyCfg.name, err)
			trySendDefaultValue(sender, subkey, gaugeName)
		} else if errors.Is(err, fs.ErrPermission) {
			log.Errorf("access denied while accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
			trySendDefaultValue(sender, subkey, gaugeName)
		} else if errors.Is(err, registry.ErrShortBuffer) || err == nil {
			switch valueType {
			case registry.DWORD:
				fallthrough
			case registry.QWORD:
				val, _, err := regKey.GetIntegerValue(valueName)
				if err != nil {
					log.Errorf("error accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
					trySendDefaultValue(sender, subkey, gaugeName)
					continue
				}
				sender.Gauge(gaugeName, float64(val), "", nil)
			case registry.SZ:
				fallthrough
			case registry.EXPAND_SZ: // Should we expand the references to environment variables ?
				val, _, err := regKey.GetStringValue(valueName)
				if err != nil {
					log.Errorf("error accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
					trySendDefaultValue(sender, subkey, gaugeName)
					continue
				}
				// First try to parse the value into a float64
				if parsedVal, err := strconv.ParseFloat(val, 64); err == nil {
					sender.Gauge(gaugeName, parsedVal, "", nil)
				} else {
					log.Warnf("value %s of key %s cannot be parsed", valueName, regKeyCfg.originalKeyPath)
					trySendDefaultValue(sender, subkey, gaugeName)
				}
			default:
				log.Warnf("unsupported data type of value %s for key %s: %d", valueName, regKeyCfg.originalKeyPath, valueType)
				trySendDefaultValue(sender, subkey, gaugeName)
			}
		}
	}
}

func trySendDefaultValue(sender sender.Sender, subkey subkeyCfg, gaugeName string) {
	if defaultVal, exists := subkey.DefaultValue.Get(); exists {
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
