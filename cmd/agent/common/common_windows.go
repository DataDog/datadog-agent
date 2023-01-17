// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"github.com/cihub/seelog"
	"golang.org/x/sys/windows/registry"
)

var (
	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath = filepath.Join(_here, "..", "checks.d")
	distPath     string
	// ViewsPath holds the path to the folder containing the GUI support files
	viewsPath string
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
