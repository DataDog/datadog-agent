// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package winregistryimpl contains the implementation of the Windows Registry check
package winregistryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/checks/winregistry"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	agentLog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	yy "github.com/ghodss/yaml"
	"github.com/swaggest/jsonschema-go"
	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/fx"
	"golang.org/x/sys/windows/registry"
	"gopkg.in/yaml.v2"
	"io/fs"
	"strconv"
	"strings"
)

const (
	checkName   = "windows_registry" // this appears in the Agent Manager and Agent status
	checkPrefix = "winregistry"      // This is the prefix used for all metrics emitted by this check
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newWindowsRegistryComponent))
}

type dependencies struct {
	fx.In

	// Logs Agent component, used to send integration logs
	// It is optional because the Logs Agent can be disabled
	LogsComponent optional.Option[agent.Component]

	// Datadog Agent logs component, used to log to the Agent logs
	Log       log.Component
	Lifecycle fx.Lifecycle
}

type registryValueCfg struct {
	Name         string                   `json:"name" yaml:"name" required:"true"` // The metric name of the registry value
	DefaultValue optional.Option[float64] `json:"default_value" yaml:"default_value"`
	Mappings     []map[string]float64     `json:"mapping" yaml:"mapping"`
}

type registryKeyCfg struct {
	Name           string                      `json:"name" yaml:"name" required:"true"`                                                     // The metric name of the registry key
	RegistryValues map[string]registryValueCfg `json:"registry_values" yaml:"registry_values" minItems:"1" nullable:"false" required:"true"` // The map key is the registry value name
}

// checkCfg is the config that is specific to each check instance
type checkCfg struct {
	RegistryKeys map[string]registryKeyCfg `json:"registry_keys" yaml:"registry_keys" nullable:"false" required:"true"`
	SendOnStart  optional.Option[bool]     `json:"send_on_start" yaml:"send_on_start"`
}

// checkInitCfg is the config that is common to all check instances
type checkInitCfg struct {
	SendOnStart optional.Option[bool] `yaml:"send_on_start"`
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
	senderManager           sender.SenderManager
	sender                  sender.Sender
	logsComponent           agent.Component
	log                     log.Component
	registryKeys            []registryKey
	registryDelegate        registryDelegate
	integrationLogsDelegate *integrationLogsRegistryDelegate
}

func createOptionMapping[T any](reflector *jsonschema.Reflector, sourceType jsonschema.SimpleType) {
	option := jsonschema.Schema{}
	option.AddType(sourceType)
	reflector.AddTypeMapping(optional.Option[T]{}, option)
}

func createSchema() ([]byte, error) {
	reflector := jsonschema.Reflector{}
	createOptionMapping[bool](&reflector, jsonschema.Boolean)
	createOptionMapping[float64](&reflector, jsonschema.Number)
	schema, err := reflector.Reflect(checkCfg{})
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(schema, "", " ")
}

// Configure configures the check
func (c *WindowsRegistryCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	c.senderManager = senderManager
	c.BuildID(integrationConfigDigest, data, initConfig)
	err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	schemaString, err := createSchema()
	if err != nil {
		agentLog.Errorf("failed to create validation schema: %s", err)
		return err
	}
	schemaLoader := gojsonschema.NewBytesLoader(schemaString)
	rawDocument, err := yy.YAMLToJSON(data)
	if err != nil {
		agentLog.Errorf("failed to load the config to JSON: %s", err)
		return err
	}
	documentLoader := gojsonschema.NewBytesLoader(rawDocument)
	result, _ := gojsonschema.Validate(schemaLoader, documentLoader)
	if !result.Valid() {
		for _, err := range result.Errors() {
			if err.Value() != nil {
				agentLog.Errorf("configuration error: %s", err)
			} else {
				agentLog.Errorf("configuration error: %s (%v)", err, err.Value())
			}
		}
		return fmt.Errorf("configuration validation failed")
	}

	var initCfg checkInitCfg
	if err := yaml.Unmarshal(initConfig, &initCfg); err != nil {
		agentLog.Errorf("cannot unmarshal shared configuration: %s", err)
		return err
	}

	var conf checkCfg
	if err := yaml.Unmarshal(data, &conf); err != nil {
		agentLog.Errorf("cannot unmarshal configuration: %s", err)
		return err
	}

	var sendOnStart, sendOnStartSet bool
	if sendOnStart, sendOnStartSet = conf.SendOnStart.Get(); !sendOnStartSet {
		if initSendOnStart, initSendOnStartSet := initCfg.SendOnStart.Get(); initSendOnStartSet {
			sendOnStart = initSendOnStart
		} else {
			sendOnStart = false
		}
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
			err = fmt.Errorf("the key %s is too short to be a valid key", regKey)
			agentLog.Errorf("configuration error: %s", err)
			return err
		}

		if len(regKeyConfig.Name) == 0 {
			c.log.Warnf("the key %s does not have a metric name, skipping", regKey)
			continue
		}

		if hive, found := hiveMap[splitKeypath[0]]; found {
			regValues := make(map[string]registryValueCfg)
			for valueName, regValueCfg := range regKeyConfig.RegistryValues {
				if len(regValueCfg.Name) == 0 {
					c.log.Warnf("the subkey %s of %s does not have a metric name, skipping", valueName, regKey)
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
			err = fmt.Errorf("unknown hive %s", splitKeypath[0])
			agentLog.Errorf("configuration error: %s", err)
			return err
		}
	}

	c.sender, err = c.GetSender()
	if err != nil {
		agentLog.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
		return err
	}
	c.sender.FinalizeCheckServiceTag()

	c.integrationLogsDelegate = &integrationLogsRegistryDelegate{
		logsComponent: c.logsComponent,
		valueMap:      make(map[string]interface{}),
		origin: message.NewOrigin(sources.NewLogSource("Windows registry check", &logsConfig.LogsConfig{
			Source:  checkName,
			Service: checkName,
		})),
		// When sendOnStart is enabled, we unmute the integrations logs sender
		// which will produce a "key_created" event for the existing keys in the registry
		// during the first check run.
		// Otherwise, the integrations logs sender will get unmuted on the subsequent check runs.
		muted: !sendOnStart,
	}

	c.registryDelegate = compositeRegistryDelegate{
		registryDelegates: []registryDelegate{
			loggingRegistryDelegate{
				log: c.log,
			},
			metricsRegistryDelegate{
				sender: c.sender,
			},
			c.integrationLogsDelegate,
		},
	}

	return nil
}

func (c *WindowsRegistryCheck) processRegistryValues(regDelegate registryDelegate, regKey registry.Key, regKeyCfg registryKey) {
	for valueName, regValueCfg := range regKeyCfg.registryValues {
		var err error
		var valueType uint32
		// regKey == 0 means parent key didn't exist, but we want to emit missing metric for each of its values
		if regKey == 0 {
			err = registry.ErrNotExist
		} else {
			_, valueType, err = regKey.GetValue(valueName, nil)
		}
		if errors.Is(err, registry.ErrNotExist) {
			regDelegate.onMissing(valueName, regKeyCfg, regValueCfg, err)
		} else if errors.Is(err, fs.ErrPermission) {
			regDelegate.onAccessDenied(valueName, regKeyCfg, regValueCfg, err)
		} else if errors.Is(err, registry.ErrShortBuffer) || err == nil {
			switch valueType {
			case registry.DWORD:
				fallthrough
			case registry.QWORD:
				val, _, err := regKey.GetIntegerValue(valueName)
				if err != nil {
					regDelegate.onRetrievalError(valueName, regKeyCfg, regValueCfg, err)
					continue
				}
				regDelegate.onSendNumber(valueName, float64(val), regKeyCfg, regValueCfg)
			case registry.SZ:
				fallthrough
			case registry.EXPAND_SZ:
				val, _, err := regKey.GetStringValue(valueName)
				if err != nil {
					regDelegate.onRetrievalError(valueName, regKeyCfg, regValueCfg, err)
					continue
				}
				if valueType == registry.EXPAND_SZ {
					val, err = registry.ExpandString(val)
				}
				if err != nil {
					regDelegate.onRetrievalError(valueName, regKeyCfg, regValueCfg, err)
					continue
				}
				// First try to parse the value into a float64
				if parsedVal, err := strconv.ParseFloat(val, 64); err == nil {
					regDelegate.onSendNumber(valueName, parsedVal, regKeyCfg, regValueCfg)
				} else {
					// Value can't be parsed, let's check the mappings
					var mappingFound = false
					for _, mapping := range regValueCfg.Mappings {
						if mappedValue, found := mapping[val]; found {
							regDelegate.onSendMappedNumber(valueName, val, mappedValue, regKeyCfg, regValueCfg)
							mappingFound = true
							break
						}
					}
					if !mappingFound {
						regDelegate.onNoMappingFound(valueName, val, regKeyCfg, regValueCfg)
					}
				}
			default:
				regDelegate.onUnsupportedDataType(valueName, valueType, regKeyCfg, regValueCfg)
			}
		}
	}
}

func (c *WindowsRegistryCheck) processRegistryKeys(regDelegate registryDelegate) {
	for _, regKeyCfg := range c.registryKeys {
		regKey, err := registry.OpenKey(regKeyCfg.hive, regKeyCfg.keyPath, registry.QUERY_VALUE)
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				// Treat access denied as errors
				c.log.Errorf("access denied while accessing key %s: %s", regKeyCfg.originalKeyPath, err)
			} else if errors.Is(err, registry.ErrNotExist) {
				c.log.Warnf("key %s was not found: %s", regKeyCfg.originalKeyPath, err)
				// Process registryValues too so that we can emit missing values for each registryValues
				c.processRegistryValues(regDelegate, regKey, regKeyCfg)
			}
		} else {
			// if err == nil the key was opened, so we need to close it after we are done.
			c.processRegistryValues(regDelegate, regKey, regKeyCfg)
			regKey.Close()
		}
	}
}

// Run runs the check
func (c *WindowsRegistryCheck) Run() error {
	c.processRegistryKeys(c.registryDelegate)
	c.sender.Commit()
	if c.integrationLogsDelegate.muted {
		c.integrationLogsDelegate.muted = false
	}
	return nil
}

func newWindowsRegistryComponent(deps dependencies) winregistry.Component {
	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			core.RegisterCheck(checkName, optional.NewOption(func() check.Check {
				integrationLogs, _ := deps.LogsComponent.Get()
				return &WindowsRegistryCheck{
					CheckBase:     core.NewCheckBase(checkName),
					logsComponent: integrationLogs,
					log:           deps.Log,
				}
			}))
			return nil
		},
	})
	return struct{}{}
}
