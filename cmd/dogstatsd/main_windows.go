// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	yaml "gopkg.in/yaml.v2"
)

var (
	elog           debug.Log
	defaultLogFile = "c:\\programdata\\datadog\\logs\\dogstatsd.log"

	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog"

	enabledVals = map[string]bool{"yes": true, "true": true, "1": true,
		"no": false, "false": false, "0": false}
	subServices = map[string]string{"logs_enabled": "logs_enabled",
		"apm_enabled":     "apm_config.enabled",
		"process_enabled": "process_config.enabled"}
)

func init() {
	pd, err := winutil.GetProgramDataDirForProduct("Datadog Dogstatsd")
	if err == nil {
		DefaultConfPath = pd
		defaultLogFile = filepath.Join(pd, "logs", "dogstatsd.log")
	} else {
		winutil.LogEventViewer(ServiceName, 0x8000000F, defaultLogFile)
	}
}

// ServiceName is the name of the service in service control manager
const ServiceName = "dogstatsd"

// EnableLoggingToFile -- set up logging to file

func main() {
	config.Datadog.AddConfigPath(DefaultConfPath)

	// go_expvar server
	go http.ListenAndServe(
		fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_stats_port")),
		http.DefaultServeMux)

	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
	}
	if !isIntSess {
		confPath = DefaultConfPath
		runService(false)
		return
	}
	defer log.Flush()

	if err = dogstatsdCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	log.Infof("Service control function")
	importRegistryConfig()
	mainCtx, mainCtxCancel, err := runAgent()

	if err != nil {
		log.Errorf("Failed to start agent %v", err)
		elog.Error(0xc0000008, err.Error())
		errno = 1 // indicates non-successful return from handler.
		changes <- svc.Status{State: svc.Stopped}
		return
	}
	elog.Info(0x40000003, ServiceName)

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop:
				log.Info("Received stop message from service control manager")
				elog.Info(0x4000000b, ServiceName)
				break loop
			case svc.Shutdown:
				log.Info("Received shutdown message from service control manager")
				elog.Info(0x4000000c, ServiceName)
				break loop
			default:
				log.Warnf("unexpected control request #%d", c)
				elog.Warning(0xc0000005, string(c.Cmd))
			}
		}
	}
	elog.Info(0x40000006, ServiceName)
	log.Infof("Initiating service shutdown")
	changes <- svc.Status{State: svc.StopPending}
	stopAgent(mainCtx, mainCtxCancel)
	changes <- svc.Status{State: svc.Stopped}
	return
}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(ServiceName)
	} else {
		elog, err = eventlog.Open(ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	elog.Info(0x40000007, ServiceName)
	run := svc.Run

	err = run(ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, ServiceName)
}

// importRegistryConfig imports settings from Windows registry into datadog.yaml
func importRegistryConfig() error {

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		"SOFTWARE\\Datadog\\Datadog Dogstatsd",
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

	err = common.SetupConfigWithoutSecrets("", "dogstatsd")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// store the current datadog.yaml path
	datadogYamlPath := config.Datadog.ConfigFileUsed()

	if config.Datadog.GetString("api_key") != "" {
		return fmt.Errorf("%s seems to contain a valid configuration, not overwriting config",
			datadogYamlPath)
	}

	overrides := make(map[string]interface{})

	var val string

	if val, _, err = k.GetStringValue("api_key"); err == nil && val != "" {
		overrides["api_key"] = val
		log.Debug("Setting API key")
	} else {
		log.Debug("API key not found, not setting")
	}
	if val, _, err = k.GetStringValue("tags"); err == nil && val != "" {
		overrides["tags"] = strings.Split(val, ",")
		log.Debugf("Setting tags %s", val)
	} else {
		log.Debug("Tags not found, not setting")
	}
	if val, _, err = k.GetStringValue("hostname"); err == nil && val != "" {
		overrides["hostname"] = val
		log.Debugf("Setting hostname %s", val)
	} else {
		log.Debug("hostname not found in registry: using default value")
	}
	if val, _, err = k.GetStringValue("cmd_port"); err == nil && val != "" {
		cmdPortInt, err := strconv.Atoi(val)
		if err != nil {
			log.Warnf("Not setting api port, invalid configuration %s %v", val, err)
		} else if cmdPortInt <= 0 || cmdPortInt > 65534 {
			log.Warnf("Not setting api port, invalid configuration %s", val)
		} else {
			overrides["cmd_port"] = cmdPortInt
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
	} else {
		log.Debug("proxy key not found, not setting proxy config")
	}
	if val, _, err = k.GetStringValue("site"); err == nil && val != "" {
		overrides["site"] = val
		log.Debugf("Setting site to %s", val)
	}
	if val, _, err = k.GetStringValue("dd_url"); err == nil && val != "" {
		overrides["dd_url"] = val
		log.Debugf("Setting dd_url to %s", val)
	}
	if val, _, err = k.GetStringValue("logs_dd_url"); err == nil && val != "" {
		overrides["logs_config.logs_dd_url"] = val
		log.Debugf("Setting logs_config.dd_url to %s", val)
	}
	if val, _, err = k.GetStringValue("process_dd_url"); err == nil && val != "" {
		overrides["process_config.process_dd_url"] = val
		log.Debugf("Setting process_config.process_dd_url to %s", val)
	}
	if val, _, err = k.GetStringValue("trace_dd_url"); err == nil && val != "" {
		overrides["apm_config.apm_dd_url"] = val
		log.Debugf("Setting apm_config.apm_dd_url to %s", val)
	}
	if val, _, err = k.GetStringValue("py_version"); err == nil && val != "" {
		overrides["python_version"] = val
		log.Debugf("Setting python version to %s", val)
	}

	// apply overrides to the config
	config.AddOverrides(overrides)

	// build the global agent configuration
	err = common.SetupConfigWithoutSecrets("", "dogstatsd")
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

	valuenames := []string{"api_key", "tags", "hostname",
		"proxy_host", "proxy_port", "proxy_user", "proxy_password", "cmd_port"}
	for _, valuename := range valuenames {
		k.DeleteValue(valuename)
	}
	for valuename := range subServices {
		k.DeleteValue(valuename)
	}
	log.Debugf("Successfully wrote the config into %s\n", datadogYamlPath)

	return nil
}
