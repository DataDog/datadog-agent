// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"encoding/json"
	"expvar"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"

	"github.com/DataDog/datadog-agent/cmd/agent/common"

	"github.com/jhoonb/archivex"
	yaml "gopkg.in/yaml.v2"
)


// SearchPaths is just an alias for a map of strings
type SearchPaths map[string]string

// CreateArchive packages up the files
func CreateArchive(local, troubleshooting bool, distPath, pyChecksPath, logFilePath string) (string, error) {
	zipFilePath := mkFilePath()
	confSearchPaths := SearchPaths{
		"":        config.Datadog.GetString("confd_path"),
		"dist":    filepath.Join(distPath, "conf.d"),
		"checksd": pyChecksPath,
	}
	return createArchive(zipFilePath, local, troubleshooting, confSearchPaths, logFilePath)
}

func createArchive(zipFilePath string, local, troubleshooting bool, confSearchPaths SearchPaths, logFilePath string) (string, error) {
	zipFile := new(archivex.ZipFile)
	zipFile.Create(zipFilePath)

	// Get hostname, if there's an error in getting the hostname,
	// set the hostname to unknown
	hostname, err := util.GetHostname()
	if err != nil {
		hostname = "unknown"
	}

	defer zipFile.Close()

	if local {
		zipFile.Add(filepath.Join(hostname, "local"), []byte{})
	} else {
		// The Status will be unavailable unless the agent is running.
		// Only zip it up if the agent is running
		err = zipStatusFile(zipFile, hostname)
		if err != nil {
			return "", err
		}
	}

	err = zipLogFiles(zipFile, hostname, logFilePath)
	if err != nil {
		return "", err
	}

	err = zipConfigFiles(zipFile, hostname, confSearchPaths)
	if err != nil {
		return "", err
	}

	err = zipExpVar(zipFile, hostname)
	if err != nil {
		return "", err
	}

	err = zipDiagnose(zipFile, hostname)
	if err != nil {
		return "", err
	}
    if troubleshooting {
		err = zipTroubleshoot(zipFile, hostname)
		if err != nil {
			return "", err
		}
	}
	if config.IsContainerized() {
		err = zipDockerSelfInspect(zipFile, hostname)
		if err != nil {
			return "", err
		}
	}
	return zipFilePath, nil
}

func zipStatusFile(zipFile *archivex.ZipFile, hostname string) error {
	// Grab the status
	s, err := status.GetAndFormatStatus()
	if err != nil {
		return err
	}
	// Clean it up
	cleaned, err := credentialsCleanerBytes(s)
	if err != nil {
		return err
	}
	// Add it to the zipfile
	zipFile.Add(filepath.Join(hostname, "status.log"), cleaned)
	return err
}

func zipLogFiles(zipFile *archivex.ZipFile, hostname, logFilePath string) error {
	logFileDir := path.Dir(logFilePath)
	err := filepath.Walk(logFileDir, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			return nil
		}
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".log" || getFirstSuffix(f.Name()) == ".log" {
			fileName := filepath.Join(hostname, "logs", f.Name())
			return zipFile.AddFileWithName(fileName, path)
		}
		return nil
	})

	return err
}

func zipTroubleshoot(zipFile *archivex.ZipFile, hostname string) error {
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
	checks := common.AC.GetConfigChecks()
	for _, check := range checks {
		result, err := check.Troubleshoot()
		err = zipFile.Add(filepath.Join(hostname, "troubleshoot", check.String()), []byte(result))
		if err != nil {
			return err
		}
	}

	return nil
}

func zipExpVar(zipFile *archivex.ZipFile, hostname string) error {
	var variables = make(map[string]interface{})
	expvar.Do(func(kv expvar.KeyValue) {
		var variable = make(map[string]interface{})
		json.Unmarshal([]byte(kv.Value.String()), &variable)
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
		cleanedYAML, err := credentialsCleanerBytes(yamlValue)
		if err != nil {
			return err
		}
		err = zipFile.Add(filepath.Join(hostname, "expvar", key), cleanedYAML)
		if err != nil {
			return err
		}
	}

	return nil
}

func zipConfigFiles(zipFile *archivex.ZipFile, hostname string, confSearchPaths SearchPaths) error {
	c, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		return err
	}
	// zip up the actual config
	cleaned, err := credentialsCleanerBytes(c)
	if err != nil {
		return err
	}
	err = zipFile.Add(filepath.Join(hostname, "datadog.yaml_in_mem"), cleaned)
	if err != nil {
		return err
	}

	err = walkConfigFilePaths(zipFile, hostname, confSearchPaths)
	if err != nil {
		return err
	}

	if config.Datadog.ConfigFileUsed() != "" {
		// zip up the config file that was actually used, if one exists
		filePath := config.Datadog.ConfigFileUsed()
		// Check if the file exists
		_, e := os.Stat(filePath)
		if e == nil {
			file, e := credentialsCleanerFile(filePath)
			if err != nil {
				return e
			}
			fileName := filepath.Join(hostname, "etc", "datadog.yaml_on_disk")
			e = zipFile.Add(fileName, file)
			if e != nil {
				return e
			}
		}
	}

	return err
}

func zipDiagnose(zipFile *archivex.ZipFile, hostname string) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	diagnose.RunAll(writer)
	writer.Flush()

	err := zipFile.Add(filepath.Join(hostname, "diagnose.log"), b.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func walkConfigFilePaths(zipFile *archivex.ZipFile, hostname string, confSearchPaths SearchPaths) error {
	for prefix, filePath := range confSearchPaths {
		err := filepath.Walk(filePath, func(path string, f os.FileInfo, err error) error {
			if f == nil {
				return nil
			}
			if f.IsDir() {
				return nil
			}

			if filepath.Ext(f.Name()) == ".example" {
				return nil
			}

			if getFirstSuffix(f.Name()) == ".yaml" || filepath.Ext(f.Name()) == ".yaml" {
				baseName := strings.Replace(path, filePath, "", 1)
				fileName := filepath.Join(hostname, "etc", "confd", prefix, baseName)
				file, err := credentialsCleanerFile(path)
				if err != nil {
					return err
				}
				return zipFile.Add(fileName, file)
			}

			return nil
		})

		if err != nil {
			return err
		}

	}

	return nil
}

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}

func mkFilePath() string {
	dir := os.TempDir()
	t := time.Now()
	timeString := t.Format("2006-01-02-15-04-05")
	fileName := strings.Join([]string{"datadog", "agent", timeString}, "-")
	fileName = strings.Join([]string{fileName, "zip"}, ".")
	filePath := path.Join(dir, fileName)
	return filePath
}
