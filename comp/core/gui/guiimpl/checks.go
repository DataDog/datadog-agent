// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/gorilla/mux"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	configPaths = []string{
		config.Datadog.GetString("confd_path"),      // Custom checks
		filepath.Join(path.GetDistPath(), "conf.d"), // Default check configs
	}

	checkPaths = []string{
		filepath.Join(path.GetDistPath(), "checks.d"),  // Custom checks
		config.Datadog.GetString("additional_checksd"), // Custom checks
		path.PyChecksPath, // Integrations-core checks
	}
)

// Adds the specific handlers for /checks/ endpoints
func checkHandler(r *mux.Router, collector collector.Component, ac autodiscovery.Component) {
	r.HandleFunc("/running", http.HandlerFunc(sendRunningChecks)).Methods("POST")
	r.HandleFunc("/run/{name}", http.HandlerFunc(runCheckHandler(collector, ac))).Methods("POST")
	r.HandleFunc("/run/{name}/once", http.HandlerFunc(runCheckOnceHandler(ac))).Methods("POST")
	r.HandleFunc("/reload/{name}", http.HandlerFunc(reloadCheckHandler(collector, ac))).Methods("POST")
	r.HandleFunc("/getConfig/{fileName}", http.HandlerFunc(getCheckConfigFile)).Methods("POST")
	r.HandleFunc("/getConfig/{checkFolder}/{fileName}", http.HandlerFunc(getCheckConfigFile)).Methods("POST")
	r.HandleFunc("/setConfig/{fileName}", http.HandlerFunc(setCheckConfigFile)).Methods("POST")
	r.HandleFunc("/setConfig/{checkFolder}/{fileName}", http.HandlerFunc(setCheckConfigFile)).Methods("POST")
	r.HandleFunc("/setConfig/{fileName}", http.HandlerFunc(setCheckConfigFile)).Methods("DELETE")
	r.HandleFunc("/setConfig/{checkFolder}/{fileName}", http.HandlerFunc(setCheckConfigFile)).Methods("DELETE")
	r.HandleFunc("/listChecks", http.HandlerFunc(listChecks)).Methods("POST")
	r.HandleFunc("/listConfigs", http.HandlerFunc(listConfigs)).Methods("POST")
}

// Sends a list of all the current running checks
func sendRunningChecks(w http.ResponseWriter, _ *http.Request) {
	html, e := renderRunningChecks()
	if e != nil {
		w.Write([]byte("Error generating status html: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Schedules a specific check
func runCheckHandler(collector collector.Component, ac autodiscovery.Component) func(_ http.ResponseWriter, r *http.Request) {
	return func(_ http.ResponseWriter, r *http.Request) {
		// Fetch the desired check
		name := mux.Vars(r)["name"]
		instances := pkgcollector.GetChecksByNameForConfigs(name, ac.GetAllConfigs())

		for _, ch := range instances {
			collector.RunCheck(ch) //nolint:errcheck
		}
		log.Infof("Scheduled new check: " + name)
	}
}

// runCheckOnceHandler generates a runCheckOnce handler with the autodiscovery component
func runCheckOnceHandler(
	ac autodiscovery.Component) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runCheckOnce(w, r, ac)
	}
}

// Runs a specified check once
func runCheckOnce(w http.ResponseWriter, r *http.Request, ac autodiscovery.Component) {
	response := make(map[string]string)
	// Fetch the desired check
	name := mux.Vars(r)["name"]
	instances := pkgcollector.GetChecksByNameForConfigs(name, ac.GetAllConfigs())
	if len(instances) == 0 {
		html, e := renderError(name)
		if e != nil {
			html = "Error generating html: " + e.Error()
		}

		response["success"] = "" // empty string evaluates to false in JS
		response["html"] = html
		res, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)
		return
	}

	// Run the check intance(s) once, as a test
	stats := []*checkstats.Stats{}
	for _, ch := range instances {
		s := checkstats.NewStats(ch)

		t0 := time.Now()
		err := ch.Run()
		warnings := ch.GetWarnings()
		sStats, _ := ch.GetSenderStats()
		s.Add(time.Since(t0), err, warnings, sStats)

		// Without a small delay some of the metrics will not show up
		time.Sleep(100 * time.Millisecond)

		stats = append(stats, s)
	}

	// Render the stats
	html, e := renderCheck(name, stats)
	if e != nil {
		response["success"] = ""
		response["html"] = "Error generating html: " + e.Error()
		res, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)
		return
	}

	response["success"] = "true"
	response["html"] = html
	res, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Reloads a running check
func reloadCheckHandler(collector collector.Component, ac autodiscovery.Component) func(_ http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := html.EscapeString(mux.Vars(r)["name"])
		instances := pkgcollector.GetChecksByNameForConfigs(name, ac.GetAllConfigs())
		if len(instances) == 0 {
			log.Errorf("Can't reload " + name + ": check has no new instances.")
			w.Write([]byte("Can't reload " + name + ": check has no new instances"))
			return
		}

		killed, e := collector.ReloadAllCheckInstances(name, instances)
		if e != nil {
			log.Errorf("Error reloading check: " + e.Error())
			w.Write([]byte("Error reloading check: " + e.Error()))
			return
		}

		log.Infof("Removed %v old instance(s) and started %v new instance(s) of %s", len(killed), len(instances), name)
		fmt.Fprintf(w, "Removed %v old instance(s) and started %v new instance(s) of %s", len(killed), len(instances), name)
	}
}

func getPathComponentFromRequest(vars map[string]string, name string, allowEmpty bool) (string, error) {
	val := vars[name]

	if (val == "" && allowEmpty) || (val != "" && !strings.Contains(val, "\\") && !strings.Contains(val, "/") && !strings.HasPrefix(val, ".")) {
		return val, nil
	}

	return "", errors.New("invalid path component")
}

func getFileNameAndFolder(vars map[string]string) (fileName, checkFolder string, err error) {
	if fileName, err = getPathComponentFromRequest(vars, "fileName", false); err != nil {
		return "", "", err
	}
	if checkFolder, err = getPathComponentFromRequest(vars, "checkFolder", true); err != nil {
		return "", "", err
	}
	return fileName, checkFolder, nil
}

// Sends the specified config (.yaml) file
func getCheckConfigFile(w http.ResponseWriter, r *http.Request) {
	fileName, checkFolder, err := getFileNameAndFolder(mux.Vars(r))
	if err != nil {
		w.WriteHeader(404)
		return
	}

	if checkFolder != "" {
		fileName = filepath.Join(checkFolder, fileName)
	}

	var file []byte
	var e error
	for _, path := range configPaths {
		filePath, err := securejoin.SecureJoin(path, fileName)
		if err != nil {
			log.Errorf("Error: Unable to join config path with the file name: %s", fileName)
			continue
		}
		file, e = os.ReadFile(filePath)
		if e == nil {
			break
		}
	}
	if file == nil {
		w.Write([]byte("Error: Couldn't find " + fileName))
		return
	}

	w.Header().Set("Content-Type", "text")
	w.Write(file)
}

type configFormat struct {
	ADIdentifiers []string    `yaml:"ad_identifiers"`
	InitConfig    interface{} `yaml:"init_config"`
	MetricConfig  interface{} `yaml:"jmx_metrics"`
	LogsConfig    interface{} `yaml:"logs"`
	Instances     []integration.RawMap
}

// Overwrites a specific check's configuration (yaml) file with new data
// or makes a new config file for that check, if there isn't one yet
func setCheckConfigFile(w http.ResponseWriter, r *http.Request) {
	fileName, checkFolder, err := getFileNameAndFolder(mux.Vars(r))
	if err != nil {
		w.WriteHeader(404)
		return
	}

	var checkConfFolderPath, defaultCheckConfFolderPath string

	if checkFolder != "" {
		checkConfFolderPath = filepath.Join(config.Datadog.GetString("confd_path"), checkFolder)
		defaultCheckConfFolderPath = filepath.Join(path.GetDistPath(), "conf.d", checkFolder)
	} else {
		checkConfFolderPath = config.Datadog.GetString("confd_path")
		defaultCheckConfFolderPath = filepath.Join(path.GetDistPath(), "conf.d")
	}

	if r.Method == "POST" {
		payload, e := parseBody(r)
		if e != nil {
			w.Write([]byte(e.Error()))
		}
		data := []byte(payload.Config)

		// Check that the data is actually a valid yaml file
		cf := configFormat{}
		e = yaml.Unmarshal(data, &cf)
		if e != nil {
			w.Write([]byte("Error: " + e.Error()))
			return
		}
		if cf.MetricConfig == nil && cf.LogsConfig == nil && len(cf.Instances) < 1 {
			w.Write([]byte("Configuration file contains no valid instances or log configuration"))
			return
		}

		// Attempt to write new configs to custom checks directory
		path, err := securejoin.SecureJoin(checkConfFolderPath, fileName)
		if err != nil {
			log.Errorf("Error: Unable to join conf folder path with the file name: %s", fileName)
			return
		}
		os.MkdirAll(checkConfFolderPath, os.FileMode(0755)) //nolint:errcheck
		e = os.WriteFile(path, data, 0600)

		// If the write didn't work, try writing to the default checks directory
		if e != nil && strings.Contains(e.Error(), "no such file or directory") {
			path, err = securejoin.SecureJoin(defaultCheckConfFolderPath, fileName)
			if err != nil {
				log.Errorf("Error: Unable to join conf folder path with the file name: %s", fileName)
				return
			}
			os.MkdirAll(defaultCheckConfFolderPath, os.FileMode(0755)) //nolint:errcheck
			e = os.WriteFile(path, data, 0600)
		}

		if e != nil {
			w.Write([]byte("Error saving config file: " + e.Error()))
			log.Debug("Error saving config file: " + e.Error())
			return
		}

		log.Infof("Successfully wrote new " + fileName + " config file.")
		w.Write([]byte("Success"))
	} else if r.Method == "DELETE" {
		// Attempt to write new configs to custom checks directory
		path, err := securejoin.SecureJoin(checkConfFolderPath, fileName)
		if err != nil {
			log.Errorf("Error: Unable to join conf folder path with the file name: %s", fileName)
			return
		}
		e := os.Rename(path, path+".disabled")

		// If the move didn't work, try writing to the dev checks directory
		if e != nil {
			path, err = securejoin.SecureJoin(defaultCheckConfFolderPath, fileName)
			if err != nil {
				log.Errorf("Error: Unable to join conf folder path with the file name: %s", fileName)
				return
			}
			e = os.Rename(path, path+".disabled")
		}

		if e != nil {
			w.Write([]byte("Error disabling config file: " + e.Error()))
			log.Errorf("Error disabling config file (%v): %v ", path, e)
			return
		}

		log.Infof("Successfully disabled integration " + fileName + " config file.")
		w.Write([]byte("Success"))
	}
}

func getWheelsChecks() ([]string, error) {
	pyChecks := []string{}

	// The integration list includes JMX integrations, they ship as wheels too.
	// JMX wheels just contain sample configs, but they do ship.
	integrations, err := getPythonChecks()
	if err != nil {
		return []string{}, err
	}

	for _, integration := range integrations {
		if _, ok := config.StandardJMXIntegrations[integration]; !ok {
			pyChecks = append(pyChecks, integration)
		}
	}

	return pyChecks, nil
}

// Sends a list containing the names of all the checks
func listChecks(w http.ResponseWriter, _ *http.Request) {
	integrations := []string{}
	for _, path := range checkPaths {
		files, err := os.ReadDir(path)
		if err != nil {
			continue
		}

		for _, file := range files {
			if ext := filepath.Ext(file.Name()); ext == ".py" && file.Type().IsRegular() {
				integrations = append(integrations, file.Name())
			}
		}
	}

	wheelsIntegrations, err := getWheelsChecks()
	if err != nil {
		log.Errorf("Unable to compile list of installed integrations: %v", err)
		w.Write([]byte("Unable to compile list of installed integrations."))
		return
	}

	// Get python wheels
	integrations = append(integrations, wheelsIntegrations...)

	// Get go-checks
	goIntegrations := core.GetRegisteredFactoryKeys()
	integrations = append(integrations, goIntegrations...)

	// Get jmx-checks
	for integration := range config.StandardJMXIntegrations {
		integrations = append(integrations, integration)
	}

	if len(integrations) == 0 {
		w.Write([]byte("No check (.py) files found."))
		return
	}

	res, _ := json.Marshal(integrations)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// collects the configs in the specified path
func getConfigsInPath(path string) ([]string, error) {
	filenames := []string{}

	files, e := readConfDir(path)
	if e != nil {
		return []string{}, nil
	}

	// If a default config is found but a non-default version exists, ignore default
	sort.Strings(files)
	lookup := make(map[string]bool)
	for _, file := range files {
		checkName := file[:strings.Index(file, ".")]
		fileName := filepath.Base(file)

		// metrics yaml does not contain the actual check config - skip
		match, _ := filepath.Match(fileName, "metrics.yaml")
		if checkName != "metrics" && match {
			continue
		}

		if ext := filepath.Ext(fileName); ext == ".default" {
			if _, exists := lookup[checkName]; exists {
				continue
			}
		}

		filenames = append(filenames, file)
		lookup[checkName] = true
	}
	return filenames, nil
}

// Sends a list containing the names of all the config files
func listConfigs(w http.ResponseWriter, _ *http.Request) {
	filenames := []string{}
	for _, path := range configPaths {

		configs, e := getConfigsInPath(path)
		if e != nil {
			log.Errorf("Unable to list configurations from %s: %v", path, e)
			continue
		}
		filenames = append(filenames, configs...)
	}

	if len(filenames) == 0 {
		w.Write([]byte("No configuration (.yaml) files found."))
		return
	}

	res, _ := json.Marshal(filenames)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Helper function which returns all the filenames in a check config directory
func readConfDir(path string) ([]string, error) {
	var filenames []string
	entries, err := os.ReadDir(path)
	if err != nil {
		return filenames, err
	}

	for _, entry := range entries {
		// Some check configs are in nested subdirectories
		if entry.IsDir() {
			if filepath.Ext(entry.Name()) != ".d" {
				continue
			}

			subEntries, err := os.ReadDir(filepath.Join(path, entry.Name()))
			if err == nil {
				for _, subEntry := range subEntries {
					if hasRightEnding(subEntry.Name()) && subEntry.Type().IsRegular() {
						// Save the full path of the config file {check_name.d}/{filename}
						filenames = append(filenames, entry.Name()+"/"+subEntry.Name())
					}
				}
			}
			continue
		}

		if hasRightEnding(entry.Name()) && entry.Type().IsRegular() {
			filenames = append(filenames, entry.Name())
		}
	}

	return filenames, nil
}

// Helper function which checks if a file has a valid extension
func hasRightEnding(filename string) bool {
	// Only accept files of the format
	//	{name}.yaml, {name}.yml
	//	{name}.yaml.default, {name}.yml.default
	//	{name}.yaml.disabled, {name}.yml.disabled
	//	{name}.yaml.example, {name}.yml.example

	ext := filepath.Ext(filename)
	if ext == ".default" {
		ext = filepath.Ext(strings.TrimSuffix(filename, ".default"))
	} else if ext == ".disabled" {
		ext = filepath.Ext(strings.TrimSuffix(filename, ".disabled"))
	} else if ext == ".example" {
		ext = filepath.Ext(strings.TrimSuffix(filename, ".example"))
	}

	return ext == ".yaml" || ext == ".yml"
}
