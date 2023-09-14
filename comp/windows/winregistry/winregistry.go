// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winregistry

import (
	"context"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util"
	agentLog "github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"golang.org/x/sys/windows/registry"
	"gopkg.in/yaml.v2"
)

const (
	checkName   = "windows_registry" // this appears in the Agent Manager and Agent status
	checkPrefix = "winregistry"      // This is the prefix used for all metrics emitted by this check
)

type dependencies struct {
	fx.In

	LogsComponent util.Optional[agent.Component] // Logs Agent component
	Lifecycle     fx.Lifecycle
}

type subkeyCfg struct {
	Name  string // The metric name of the registry value
	Value string // The registry key value
}

type registryKeyCfg struct {
	Name    string               // The metric name of the registry key
	Subkeys map[string]subkeyCfg // The map key is the subkey name
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
	logsComponent agent.Component
	registryKeys  []registryKey
	origin        *message.Origin
}

func (c *WindowsRegistryCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	c.origin = message.NewOrigin(sources.NewLogSource("Windows registry check", &logsConfig.LogsConfig{
		Source: checkName,
		//Service: "my_custom_service",
		//Tags:    []string{"foo:bar", "custom:log"},
	}))

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
			return agentLog.Errorf("the key %s is too short to be a valid key", regKey)
		}

		if hive, found := hiveMap[splitKeypath[0]]; found {
			c.registryKeys = append(c.registryKeys, registryKey{
				name:            regKeyConfig.Name,
				hive:            hive,
				originalKeyPath: regKey,
				keyPath:         strings.Join(splitKeypath[1:], "\\"),
				subkeys:         regKeyConfig.Subkeys,
			})
		} else {
			return agentLog.Errorf("unknown hive %s", splitKeypath[0])
		}
	}

	return nil
}

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
				c.errorf("access denied while accessing key %s: %s", regKeyCfg.originalKeyPath, err)
			} else if errors.Is(err, registry.ErrNotExist) {
				c.warnf("key %s was not found: %s", regKeyCfg.originalKeyPath, err)
			}
		} else {
			// if err == nil the key was opened, so we need to close it after we are done.
			c.processRegistryKeyMetrics(sender, regKey, regKeyCfg)
			regKey.Close()
		}
	}
	sender.Commit()
	return nil
}

func (c *WindowsRegistryCheck) emitLog(m, status string) {
	if !c.logsComponent.IsRunning() {
		return
	}
	c.logsComponent.GetPipelineProvider().NextPipelineChan() <- message.NewMessage(
		[]byte(m),
		c.origin,
		status,
		time.Now().UnixNano(),
	)
}

func (c *WindowsRegistryCheck) warnf(format string, params ...interface{}) {
	if c.logsComponent.IsRunning() {
		c.emitLog(fmt.Sprintf(format, params), "warn")
	} else {
		agentLog.Warnf(format, params)
	}
}

func (c *WindowsRegistryCheck) infof(format string, params ...interface{}) {
	if c.logsComponent.IsRunning() {
		c.emitLog(fmt.Sprintf(format, params), "info")
	} else {
		agentLog.Infof(format, params)
	}
}

func (c *WindowsRegistryCheck) errorf(format string, params ...interface{}) {
	if c.logsComponent.IsRunning() {
		c.emitLog(fmt.Sprintf(format, params), "error")
	} else {
		agentLog.Errorf(format, params)
	}
}

func (c *WindowsRegistryCheck) processRegistryKeyMetrics(sender sender.Sender, regKey registry.Key, regKeyCfg registryKey) {
	for valueName, subkey := range regKeyCfg.subkeys {
		_, valueType, err := regKey.GetValue(valueName, nil)
		gaugeName := fmt.Sprintf("%s.%s.%s", checkPrefix, regKeyCfg.name, subkey.Name)
		if errors.Is(err, registry.ErrNotExist) {
			c.warnf("value %s of key %s was not found: %s", valueName, regKeyCfg.name, err)
		} else if errors.Is(err, fs.ErrPermission) {
			c.errorf("access denied while accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
		} else if errors.Is(err, registry.ErrShortBuffer) || err == nil {
			switch valueType {
			case registry.DWORD:
				fallthrough
			case registry.QWORD:
				val, _, err := regKey.GetIntegerValue(valueName)
				if err != nil {
					c.errorf("error accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
					continue
				}
				sender.Gauge(gaugeName, float64(val), "", nil)
				sVal := fmt.Sprintf("%f", float64(val))
				if subkey.Value != sVal {
					c.emitLog(fmt.Sprintf("%s\\%s changed from %s to %s", regKeyCfg.name, subkey.Name, subkey.Value, sVal), "info")
					subkey.Value = sVal
				}
			case registry.SZ:
				fallthrough
			case registry.MULTI_SZ:
				fallthrough
			case registry.EXPAND_SZ: // Should we expand the references to environment variables ?
				val, _, err := regKey.GetStringValue(valueName)
				if err != nil {
					c.errorf("error accessing value %s of key %s: %s", valueName, regKeyCfg.originalKeyPath, err)
					continue
				}
				// Try to parse the value into a float64
				if parsedVal, err := strconv.ParseFloat(val, 64); err == nil {
					sender.Gauge(gaugeName, parsedVal, "", nil)
					if subkey.Value != val {
						c.emitLog(fmt.Sprintf("%s\\%s changed from %s to %s", regKeyCfg.name, subkey.Name, subkey.Value, val), "info")
						subkey.Value = val
					}
				} else {
					c.warnf("value %s of key %s cannot be parsed ", valueName, regKeyCfg.originalKeyPath)
				}
			default:
				c.warnf("unsupported data type of value %s for key %s: %d", valueName, regKeyCfg.originalKeyPath, valueType)
			}
		}
	}
}

func newWindowsRegistry(deps dependencies) Component {
	logs, _ := deps.LogsComponent.Get()
	instance := &WindowsRegistryCheck{
		logsComponent: logs,
		CheckBase:     core.NewCheckBase(checkName),
	}
	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			core.RegisterCheck(checkName, func() check.Check {
				return instance
			})
			return nil
		},
	})
	return instance
}
