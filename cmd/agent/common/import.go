// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/legacy"
)

// TransformationFunc type represents transformation applicable to byte slices
type TransformationFunc func(rawData []byte) ([]byte, error)

// ImportConfig imports the agent5 configuration into the agent6 yaml config
func ImportConfig(oldConfigDir string, newConfigDir string, force bool) error {
	datadogConfPath := filepath.Join(oldConfigDir, "datadog.conf")
	datadogYamlPath := filepath.Join(newConfigDir, "datadog.yaml")
	traceAgentConfPath := filepath.Join(newConfigDir, "trace-agent.conf")
	configConverter := config.NewConfigConverter()
	const cfgExt = ".yaml"
	const dirExt = ".d"

	// read the old configuration in memory
	agentConfig, err := legacy.GetAgentConfig(datadogConfPath)
	if err != nil {
		return fmt.Errorf("unable to read data from %s: %v", datadogConfPath, err)
	}

	// the new config file might not exist, create it
	created := false
	if _, err := os.Stat(datadogYamlPath); os.IsNotExist(err) {
		f, err := os.Create(datadogYamlPath)
		if err != nil {
			return fmt.Errorf("error creating %s: %v", datadogYamlPath, err)
		}
		f.Close()
		created = true
	}

	// setup the configuration system
	config.Datadog().AddConfigPath(newConfigDir)
	_, err = config.LoadWithoutSecret()
	if err != nil {
		return fmt.Errorf("unable to load Datadog config file: %s", err)
	}

	// we won't overwrite the conf file if it contains a valid api_key
	if config.Datadog().GetString("api_key") != "" && !force {
		return fmt.Errorf("%s seems to contain a valid configuration, run the command again with --force or -f to overwrite it",
			datadogYamlPath)
	}

	// merge current agent configuration with the converted data
	err = legacy.FromAgentConfig(agentConfig, configConverter)
	if err != nil {
		return fmt.Errorf("unable to convert configuration data from %s: %v", datadogConfPath, err)
	}

	// move existing config files to the new configuration directory
	files, err := os.ReadDir(filepath.Join(oldConfigDir, "conf.d"))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(color.Output,
				"%s does not exist, no config files to import.\n",
				color.BlueString(filepath.Join(oldConfigDir, "conf.d")),
			)
		} else {
			return fmt.Errorf("unable to list config files from %s: %v", oldConfigDir, err)
		}
	}

	tr := []TransformationFunc{relocateMinCollectionInterval}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != cfgExt {
			continue
		}
		checkName := strings.TrimSuffix(f.Name(), cfgExt)

		src := filepath.Join(oldConfigDir, "conf.d", f.Name())
		dst := filepath.Join(newConfigDir, "conf.d", checkName+dirExt, "conf"+cfgExt)

		if f.Name() == "docker_daemon.yaml" {
			err := legacy.ImportDockerConf(src, filepath.Join(newConfigDir, "conf.d", "docker.d", "conf.yaml"), force, configConverter)
			if err != nil {
				return err
			}
			continue
		} else if f.Name() == "docker.yaml" {
			// if people upgrade from a very old version of the agent who ship the old docker check.
			fmt.Fprintf(
				color.Output,
				"Ignoring %s, old docker check has been deprecated.\n", color.YellowString(src),
			)
			continue
		} else if f.Name() == "kubernetes.yaml" {
			err := legacy.ImportKubernetesConf(src, filepath.Join(newConfigDir, "conf.d", "kubelet.d", "conf.yaml"), force, configConverter)
			if err != nil {
				return err
			}
			continue
		}

		if err := copyFile(src, dst, force, tr); err != nil {
			return fmt.Errorf("unable to copy %s to %s: %v", src, dst, err)
		}

		fmt.Fprintf(
			color.Output,
			"Copied %s over the new %s directory\n",
			color.BlueString("conf.d/"+f.Name()),
			color.BlueString(checkName+dirExt),
		)
	}

	// backup the original datadog.yaml to datadog.yaml.bak
	if !created {
		err = os.Rename(datadogYamlPath, datadogYamlPath+".bak")
		if err != nil {
			return fmt.Errorf("unable to create a backup for the existing file: %s", datadogYamlPath)
		}
	}

	// marshal the config object to YAML
	b, err := yaml.Marshal(config.Datadog().AllSettings())
	if err != nil {
		return fmt.Errorf("unable to marshal config to YAML: %v", err)
	}

	// dump the current configuration to datadog.yaml
	// file permissions will be used only to create the file if doesn't exist,
	// please note on Windows such permissions have no effect.
	if err = os.WriteFile(datadogYamlPath, b, 0640); err != nil {
		return fmt.Errorf("unable to write config to %s: %v", datadogYamlPath, err)
	}

	fmt.Fprintf(
		color.Output,
		"%s imported the contents of %s into %s\n",
		color.GreenString("Success:"),
		datadogConfPath,
		datadogYamlPath,
	)

	// move existing config templates to the new auto_conf directory
	autoConfFiles, err := os.ReadDir(filepath.Join(oldConfigDir, "conf.d", "auto_conf"))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(color.Output,
				"%s does not exist, no auto_conf files to import.\n",
				color.BlueString(filepath.Join(oldConfigDir, "conf.d", "auto_conf")),
			)
		} else {
			return fmt.Errorf("unable to list auto_conf files from %s: %v", oldConfigDir, err)
		}
	}

	for _, f := range autoConfFiles {
		if f.IsDir() || filepath.Ext(f.Name()) != cfgExt {
			continue
		}
		checkName := strings.TrimSuffix(f.Name(), cfgExt)

		src := filepath.Join(oldConfigDir, "conf.d", "auto_conf", f.Name())
		dst := filepath.Join(newConfigDir, "conf.d", checkName+dirExt, "auto_conf"+cfgExt)

		if err := copyFile(src, dst, force, tr); err != nil {
			fmt.Fprintf(os.Stderr, "unable to copy %s to %s: %v\n", src, dst, err)
			continue
		}

		// Transform if needed AD configuration
		input, err := os.ReadFile(dst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to open %s", dst)
			continue
		}
		output := strings.Replace(string(input), "docker_images:", "ad_identifiers:", 1)
		err = os.WriteFile(dst, []byte(output), 0640)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to write %s", dst)
			continue
		}

		fmt.Fprintf(
			color.Output,
			"Copied %s over the new %s directory\n",
			color.BlueString("auto_conf/"+f.Name()),
			color.BlueString(checkName+dirExt),
		)
	}

	// Extract trace-agent specific info and dump it to its own config file.
	imported, err := configTraceAgent(datadogConfPath, traceAgentConfPath, force)
	if err != nil {
		return fmt.Errorf("failed to import Trace Agent specific settings: %v", err)
	}
	if imported {
		fmt.Printf("Wrote Trace Agent specific settings to %s\n", traceAgentConfPath)
	}

	return nil
}

// Copy the src file to dst. File attributes won't be copied. Apply all TransformationFunc while copying.
func copyFile(src, dst string, overwrite bool, transformations []TransformationFunc) error {
	// if the file exists check whether we can overwrite
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		if overwrite {
			// we'll overwrite, backup the original file first
			err = os.Rename(dst, dst+".bak")
			if err != nil {
				return fmt.Errorf("unable to create a backup copy of the destination file: %v", err)
			}
		} else {
			return fmt.Errorf("destination file already exists, run the command again with --force or -f to overwrite it")
		}
	}

	// Create necessary destination directories
	err := os.MkdirAll(filepath.Dir(dst), 0750)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("unable to read file %s : %s", src, err)
	}

	for _, transformation := range transformations {
		data, err = transformation(data)
		if err != nil {
			return fmt.Errorf("unable to convert file %s : %s", src, err)
		}
	}

	os.WriteFile(dst, data, 0640) //nolint:errcheck

	ddGroup, errGroup := user.LookupGroup("dd-agent")
	ddUser, errUser := user.LookupId("dd-agent")

	// Only change the owner/group of the configuration files if we can detect the dd-agent user
	// This will not take affect on Windows/MacOS as the user is not available.
	if errGroup == nil && errUser == nil {
		ddGID, err := strconv.Atoi(ddGroup.Gid)
		if err != nil {
			return fmt.Errorf("Couldn't convert dd-agent group ID: %s into an int: %s", ddGroup.Gid, err)
		}

		ddUID, err := strconv.Atoi(ddUser.Uid)
		if err != nil {
			return fmt.Errorf("Couldn't convert dd-agent user ID: %s into an int: %s", ddUser.Uid, err)
		}

		err = os.Chown(dst, ddUID, ddGID)
		if err != nil {
			return fmt.Errorf("Couldn't change the file permissions for this check. Error: %s", err)
		}
	}

	err = os.Chmod(dst, 0640)
	if err != nil {
		return err
	}

	return nil
}

// configTraceAgent extracts trace-agent specific info and dump to its own config file
func configTraceAgent(datadogConfPath, traceAgentConfPath string, overwrite bool) (bool, error) {
	// if the file exists check whether we can overwrite
	if _, err := os.Stat(traceAgentConfPath); !os.IsNotExist(err) {
		if overwrite {
			// we'll overwrite, backup the original file first
			err = os.Rename(traceAgentConfPath, traceAgentConfPath+".bak")
			if err != nil {
				return false, fmt.Errorf("unable to create a backup for the existing file: %s", traceAgentConfPath)
			}
		} else {
			return false, fmt.Errorf("destination file %s already exists, run the command again with --force or -f to overwrite it", traceAgentConfPath)
		}
	}

	return legacy.ImportTraceAgentConfig(datadogConfPath, traceAgentConfPath)
}

func relocateMinCollectionInterval(rawData []byte) ([]byte, error) {
	data := make(map[interface{}]interface{})
	if err := yaml.Unmarshal(rawData, &data); err != nil {
		return nil, fmt.Errorf("error while unmarshalling Yaml : %v", err)
	}

	if _, ok := data["init_config"]; ok {
		if initConfig, ok := data["init_config"].(map[interface{}]interface{}); ok {
			if _, ok := initConfig["min_collection_interval"]; ok {
				if minCollectionInterval, ok := initConfig["min_collection_interval"].(int); ok {
					delete(initConfig, "min_collection_interval")
					insertMinCollectionInterval(data, minCollectionInterval)
				}
			}
		}
	}
	return yaml.Marshal(data)
}

func insertMinCollectionInterval(rawData map[interface{}]interface{}, interval int) {
	if _, ok := rawData["instances"]; ok {
		if instances, ok := rawData["instances"].([]interface{}); ok {
			for _, rawInstance := range instances {
				if instance, ok := rawInstance.(map[interface{}]interface{}); ok {
					instance["min_collection_interval"] = interval
				}
			}
		}
	}
}
