// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	// Init packages
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/providers/windows"

	"github.com/cihub/seelog"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"
)

var (
	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath = filepath.Join(_here, "..", "checks.d")
	distPath     string
	// ViewsPath holds the path to the folder containing the GUI support files
	viewsPath string

	enabledVals = map[string]bool{
		"true":  true,
		"yes":   true,
		"1":     true,
		"false": false,
		"no":    false,
		"0":     false,
	}

	// Maps which settings from the registry should be taken and placed into
	// datadog.yaml keys
	regSettings = map[string]string{
		"dd_url":         "dd_url",
		"hostname_fqdn":  "hostname_fqdn",
		"logs_dd_url":    "logs_config.logs_dd_url",
		"process_dd_url": "process_config.process_dd_url",
		"py_version":     "python_version",
		"site":           "site",
		"trace_dd_url":   "apm_config.apm_dd_url",
	}
	subServices = map[string]string{
		"apm_enabled":     "apm_config.enabled",
		"logs_enabled":    "logs_enabled",
		"process_enabled": "process_config.enabled",
	}

	ec2UseWinPrefixDetectionKey = "ec2_use_windows_prefix_detection"
)

var (
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog"
	// DefaultLogFile points to the log file that will be used if not configured
	DefaultLogFile = "c:\\programdata\\datadog\\logs\\agent.log"
	// DefaultDCALogFile points to the log file that will be used if not configured
	DefaultDCALogFile = "c:\\programdata\\datadog\\logs\\cluster-agent.log"
	//DefaultJmxLogFile points to the jmx fetch log file that will be used if not configured
	DefaultJmxLogFile = "c:\\programdata\\datadog\\logs\\jmxfetch.log"
	// DefaultCheckFlareDirectory a flare friendly location for checks to be written
	DefaultCheckFlareDirectory = "c:\\programdata\\datadog\\logs\\checks\\"
	// DefaultJMXFlareDirectory a flare friendly location for jmx command logs to be written
	DefaultJMXFlareDirectory = "c:\\programdata\\datadog\\logs\\jmxinfo\\"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultConfPath = pd
		DefaultLogFile = filepath.Join(pd, "logs", "agent.log")
		DefaultDCALogFile = filepath.Join(pd, "logs", "cluster-agent.log")
	} else {
		winutil.LogEventViewer(config.ServiceName, 0x8000000F, DefaultConfPath)
	}
}

// EnableLoggingToFile -- set up logging to file
func EnableLoggingToFile() {
	seeConfig := `
<seelog>
	<outputs>
		<rollingfile type="size" filename="c:\\ProgramData\\DataDog\\Logs\\agent.log" maxsize="1000000" maxrolls="2" />
	</outputs>
</seelog>`
	logger, _ := seelog.LoggerFromConfigAsBytes([]byte(seeConfig))
	log.ReplaceLogger(logger)
}

func getInstallPath() string {
	// fetch the installation path from the registry
	installpath := filepath.Join(_here, "..")
	var s string
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\DataDog\Datadog Agent`, registry.QUERY_VALUE)
	if err != nil {
		log.Warnf("Failed to open registry key: %s", err)
	} else {
		defer k.Close()
		s, _, err = k.GetStringValue("InstallPath")
		if err != nil {
			log.Warnf("Installpath not found in registry: %s", err)
		}
	}
	// if unable to figure out the install path from the registry,
	// just compute it relative to the executable.
	if s == "" {
		s = installpath
	}
	return s
}

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	if len(distPath) == 0 {
		var s string
		if s = getInstallPath(); s == "" {
			return ""
		}
		distPath = filepath.Join(s, `bin/agent/dist`)
	}
	return distPath
}

// GetViewsPath returns the fully qualified path to the GUI's 'views' directory
func GetViewsPath() string {
	if len(viewsPath) == 0 {
		var s string
		if s = getInstallPath(); s == "" {
			return ""
		}
		viewsPath = filepath.Join(s, "bin", "agent", "dist", "views")
		log.Debugf("ViewsPath is now %s", viewsPath)
	}
	return viewsPath
}

// CheckAndUpgradeConfig checks to see if there's an old datadog.conf, and if
// datadog.yaml is either missing or incomplete (no API key).  If so, upgrade it
func CheckAndUpgradeConfig() error {
	datadogConfPath := filepath.Join(DefaultConfPath, "datadog.conf")
	if _, err := os.Stat(datadogConfPath); os.IsNotExist(err) {
		log.Debug("Previous config file not found, not upgrading")
		return nil
	}
	config.Datadog.AddConfigPath(DefaultConfPath)
	_, err := config.Load()
	if err == nil {
		// was able to read config, check for api key
		if config.Datadog.GetString("api_key") != "" {
			log.Debug("Datadog.yaml found, and API key present.  Not upgrading config")
			return nil
		}
	}
	return ImportConfig(DefaultConfPath, DefaultConfPath, false)
}

// ImportRegistryConfig imports settings from Windows registry into datadog.yaml
//
// Config settings are placed in the registry by the Windows (MSI) installer.  The
// registry is used as an intermediate step to hold the config options in between
// the installer running and the first run of the agent.
//
// The agent will only apply these settings on a new install.  A new install is determined
// by the existence of an API key in the config file (datadog.yaml).  Existence of the API key
// indicates this is an upgrade, and the settings provided on the command line (via the registry)
// will be ignored.
//
// Lack of an API key is interpreted as a new install.  Take any/all of the options supplied
// on the command line and apply them to the config, and overwrite the configuration file
// to persist the command line options. The yaml configuration file is the single source of truth,
// and thus the registry entries created by the installer are deleted to avoid confusion.
//
// Applying command line config options is handled this way as it seems preferable to use the
// existing configuration library to read/write the config, rather than have the installer
// try to modify the configuration on the fly.  This is _also_ a legacy algorithm, as at some
// point attempts to modify the config file from the installer (via a shell executable) was
// interpreted as bad behavior by some A/V programs.
func ImportRegistryConfig() error {

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		"SOFTWARE\\Datadog\\Datadog Agent",
		registry.ALL_ACCESS)
	if err != nil {
		if err == registry.ErrNotExist {
			log.Debug("Windows installation key not found, not updating config")
			return nil
		}
		// otherwise, unexpected error
		log.Warnf("Unexpected error getting registry config %s", err.Error())
		return err
	}
	defer k.Close()

	err = SetupConfigWithoutSecrets("", "")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// store the current datadog.yaml path
	datadogYamlPath := config.Datadog.ConfigFileUsed()
	validConfigFound := false
	commandLineSettingFound := false
	if config.Datadog.GetString("api_key") != "" {
		validConfigFound = true
	}

	overrides := make(map[string]interface{})

	var val string

	if val, _, err = k.GetStringValue("api_key"); err == nil && val != "" {
		overrides["api_key"] = val
		log.Debug("Setting API key")
		commandLineSettingFound = true
	} else {
		log.Debug("API key not found, not setting")
	}
	if val, _, err = k.GetStringValue("tags"); err == nil && val != "" {
		overrides["tags"] = strings.Split(val, ",")
		commandLineSettingFound = true
		log.Debugf("Setting tags %s", val)
	} else {
		log.Debug("Tags not found, not setting")
	}
	if val, _, err = k.GetStringValue("hostname"); err == nil && val != "" {
		overrides["hostname"] = val
		commandLineSettingFound = true
		log.Debugf("Setting hostname %s", val)
	} else {
		log.Debug("Hostname not found in registry: using default value")
	}
	if val, _, err = k.GetStringValue("cmd_port"); err == nil && val != "" {
		cmdPortInt, err := strconv.Atoi(val)
		if err != nil {
			log.Warnf("Not setting api port, invalid configuration %s %v", val, err)
		} else if cmdPortInt <= 0 || cmdPortInt > 65534 {
			log.Warnf("Not setting api port, invalid configuration %s", val)
		} else {
			overrides["cmd_port"] = cmdPortInt
			commandLineSettingFound = true
			log.Debugf("Setting cmd_port  %d", cmdPortInt)
		}
	} else {
		log.Debug("cmd_port not found, not setting")
	}
	for key, cfg := range subServices {
		if val, _, err = k.GetStringValue(key); err == nil {
			val = strings.ToLower(val)
			if enabled, ok := enabledVals[val]; ok {
				// some of the entries require booleans, some
				// of the entries require strings.
				if enabled {
					switch cfg {
					case "logs_enabled":
						overrides[cfg] = true
					case "apm_config.enabled":
						overrides[cfg] = true
					case "process_config.enabled":
						overrides[cfg] = "true"
					}
					log.Debugf("Setting %s to true", cfg)
				} else {
					switch cfg {
					case "logs_enabled":
						overrides[cfg] = false
					case "apm_config.enabled":
						overrides[cfg] = false
					case "process_config.enabled":
						overrides[cfg] = "disabled"
					}
					log.Debugf("Setting %s to false", cfg)
				}
				commandLineSettingFound = true
			} else {
				log.Warnf("Unknown setting %s = %s", key, val)
			}
		}
	}
	if val, _, err = k.GetStringValue("proxy_host"); err == nil && val != "" {
		var u *url.URL
		if u, err = url.Parse(val); err != nil {
			log.Warnf("unable to import value of settings 'proxy_host': %v", err)
		} else {
			// set scheme if missing
			if u.Scheme == "" {
				u, _ = url.Parse("http://" + val)
			}
			if val, _, err = k.GetStringValue("proxy_port"); err == nil && val != "" {
				u.Host = u.Host + ":" + val
			}
			if user, _, _ := k.GetStringValue("proxy_user"); err == nil && user != "" {
				if pass, _, _ := k.GetStringValue("proxy_password"); err == nil && pass != "" {
					u.User = url.UserPassword(user, pass)
				} else {
					u.User = url.User(user)
				}
			}
		}
		proxyMap := make(map[string]string)
		proxyMap["http"] = u.String()
		proxyMap["https"] = u.String()
		overrides["proxy"] = proxyMap
		commandLineSettingFound = true
	} else {
		log.Debug("proxy key not found, not setting proxy config")
	}

	for winRegKey, yamlSettingKey := range regSettings {
		if val, _, err = k.GetStringValue(winRegKey); err == nil && val != "" {
			log.Debugf("Setting %s to %s", yamlSettingKey, val)
			overrides[yamlSettingKey] = val
			commandLineSettingFound = true
		}
	}

	if val, _, err = k.GetStringValue(ec2UseWinPrefixDetectionKey); err == nil && val != "" {
		val = strings.ToLower(val)
		if enabled, ok := enabledVals[val]; ok {
			overrides[ec2UseWinPrefixDetectionKey] = enabled
			log.Debugf("Setting %s to %s", ec2UseWinPrefixDetectionKey, val)
			commandLineSettingFound = true
		} else {
			log.Warnf("Unparsable boolean value for %s: %s", ec2UseWinPrefixDetectionKey, val)
		}
	}

	// We've read in the config from the registry; remove the registry keys so it's
	// not repeated on next startup

	valuenames := []string{
		"api_key",
		"cmd_port",
		"hostname",
		"proxy_host",
		"proxy_password",
		"proxy_port",
		"proxy_user",
		"tags",
		ec2UseWinPrefixDetectionKey,
	}
	for _, valuename := range valuenames {
		k.DeleteValue(valuename)
	}
	for valuename := range regSettings {
		k.DeleteValue(valuename)
	}
	for valuename := range subServices {
		k.DeleteValue(valuename)
	}

	if !commandLineSettingFound {
		log.Debugf("No installation command line entries to update")
		return nil
	}

	if validConfigFound {
		// do this check after walking through all the registry keys.  Even though
		// we aren't going to use the results, we can have a more accurate reason
		// as to why (and how important it is)
		if commandLineSettingFound {
			log.Warnf("Install command line settings ignored, valid configuration already in place")
			return fmt.Errorf("Install command line settings ignored, valid configuration already in place")
		}
		log.Debugf("Valid configuration file found,  not overwriting config")

		// already had a valid config; don't assign the overrides
		return nil
	}
	log.Debugf("Applying settings")

	// apply overrides to the config
	config.AddOverrides(overrides)

	// build the global agent configuration
	err = SetupConfigWithoutSecrets("", "")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// dump the current configuration to datadog.yaml
	b, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		log.Errorf("unable to unmarshal config to YAML: %v", err)
		return fmt.Errorf("unable to unmarshal config to YAML: %v", err)
	}
	// file permissions will be used only to create the file if doesn't exist,
	// please note on Windows such permissions have no effect.
	if err = ioutil.WriteFile(datadogYamlPath, b, 0640); err != nil {
		log.Errorf("unable to unmarshal config to %s: %v", datadogYamlPath, err)
		return fmt.Errorf("unable to unmarshal config to %s: %v", datadogYamlPath, err)
	}

	log.Debugf("Successfully wrote the config into %s\n", datadogYamlPath)

	return nil
}
