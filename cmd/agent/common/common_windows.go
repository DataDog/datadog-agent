// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package common

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"

	log "github.com/cihub/seelog"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"
)

var (
	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath = filepath.Join(_here, "..", "checks.d")
	distPath     string
	// ViewsPath holds the path to the folder containing the GUI support files
	viewsPath string
)

const (
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog"
	// DefaultLogFile points to the log file that will be used if not configured
	DefaultLogFile = "c:\\programdata\\datadog\\logs\\agent.log"
)

// EnableLoggingToFile -- set up logging to file
func EnableLoggingToFile() {
	seeConfig := `
<seelog>
	<outputs>
		<rollingfile type="size" filename="c:\\ProgramData\\DataDog\\Logs\\agent.log" maxsize="1000000" maxrolls="2" />
	</outputs>
</seelog>`
	logger, _ := log.LoggerFromConfigAsBytes([]byte(seeConfig))
	log.ReplaceLogger(logger)
}

func getInstallPath() string {
	// fetch the installation path from the registry
	installpath := filepath.Join(_here, "..")
	var s string
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\DataDog\Datadog Agent`, registry.QUERY_VALUE)
	if err != nil {
		log.Warn("Failed to open registry key %s", err)
	} else {
		defer k.Close()
		s, _, err = k.GetStringValue("InstallPath")
		if err != nil {
			log.Warn("Installpath not found in registry %s", err)
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
		log.Debug("ViewsPath is now %s", viewsPath)
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
	err := config.Datadog.ReadInConfig()
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
		log.Warn("Unexpected error getting registry config %s", err.Error())
		return err
	}
	defer k.Close()
	// Global Agent configuration
	err = SetupConfig("")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// store the current datadog.yaml path
	datadogYamlPath := config.Datadog.ConfigFileUsed()

	if config.Datadog.GetString("api_key") != "" {
		return fmt.Errorf("%s seems to contain a valid configuration, not overwriting config",
			datadogYamlPath)
	}

	var val string

	if val, _, err = k.GetStringValue("api_key"); err == nil {
		config.Datadog.Set("api_key", val)
		log.Debug("Setting API key")
	} else {
		log.Debug("API key not found, not setting")
	}
	if val, _, err = k.GetStringValue("tags"); err == nil {
		config.Datadog.Set("tags", strings.Split(val, ","))
		log.Debugf("Setting tags %s", val)
	} else {
		log.Debug("Tags not found, not setting")
	}
	if val, _, err = k.GetStringValue("hostname"); err == nil {
		config.Datadog.Set("hostname", val)
		log.Debugf("Setting hostname %s", val)
	} else {
		log.Debug("hostname not found, not setting")
	}
	if val, _, err = k.GetStringValue("proxy_host"); err == nil {
		var u *url.URL
		if u, err = url.Parse(val); err != nil {
			log.Warnf("unable to import value of settings 'proxy_host': %v", err)
		} else {
			// set scheme if missing
			if u.Scheme == "" {
				u, _ = url.Parse("http://" + val)
			}
			if val, _, err = k.GetStringValue("proxy_port"); err == nil {
				u.Host = u.Host + ":" + val
			}
			if user, _, _ := k.GetStringValue("proxy_user"); user != "" {
				if pass, _, _ := k.GetStringValue("proxy_password"); pass != "" {
					u.User = url.UserPassword(user, pass)
				} else {
					u.User = url.User(user)
				}
			}
		}
		config.Datadog.Set("proxy", u.String())
	} else {
		log.Debug("proxy key not found, not setting proxy config")
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

	valuenames := []string{"api_key", "tags", "hostname",
		"proxy_host", "proxy_port", "proxy_user", "proxy_password"}
	for _, valuename := range valuenames {
		k.DeleteValue(valuename)
	}
	log.Debugf("Successfully wrote the config into %s\n", datadogYamlPath)

	return nil
}
