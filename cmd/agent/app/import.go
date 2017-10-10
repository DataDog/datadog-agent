// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/legacy"
	yaml "gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
)

var (
	importCmd = &cobra.Command{
		Use:          "import <old_configuration_dir> <destination_dir>",
		Short:        "Import and convert configuration files from previous versions of the Agent",
		Long:         ``,
		RunE:         doImport,
		SilenceUsage: true,
	}

	force         = false
	convertDocker = false
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(importCmd)

	// local flags
	importCmd.Flags().BoolVarP(&force, "force", "f", force, "overwrite existing files")
	importCmd.Flags().BoolVarP(&convertDocker, "docker", "", convertDocker, "convert docker_daemon.yaml to the new format")
}

func doImport(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("please provide all the required arguments")
	}

	if confFilePath != "" {
		fmt.Fprintf(os.Stderr, "Please note configdir option has no effect\n")
	}

	oldConfigDir := args[0]
	newConfigDir := args[1]
	datadogConfPath := filepath.Join(oldConfigDir, "datadog.conf")
	datadogYamlPath := filepath.Join(newConfigDir, "datadog.yaml")
	traceAgentConfPath := filepath.Join(newConfigDir, "trace-agent.conf")
	processAgentConfPath := filepath.Join(newConfigDir, "process-agent.conf")

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
	config.Datadog.AddConfigPath(newConfigDir)
	err = config.Datadog.ReadInConfig()
	if err != nil {
		return fmt.Errorf("unable to load Datadog config file: %s", err)
	}

	// we won't overwrite the conf file if it contains a valid api_key
	if config.Datadog.GetString("api_key") != "" && !force {
		return fmt.Errorf("%s seems to contain a valid configuration, run the command again with --force or -f to overwrite it",
			datadogYamlPath)
	}

	// merge current agent configuration with the converted data
	err = legacy.FromAgentConfig(agentConfig)
	if err != nil {
		return fmt.Errorf("unable to convert configuration data from %s: %v", datadogConfPath, err)
	}

	// backup the original datadog.yaml to datadog.yaml.bak
	if !created {
		err = os.Rename(datadogYamlPath, datadogYamlPath+".bak")
		if err != nil {
			return fmt.Errorf("unable to create a backup for the existing file: %s", datadogYamlPath)
		}
	}

	// move existing config files to the new configuration directory
	files, err := ioutil.ReadDir(filepath.Join(oldConfigDir, "conf.d"))
	if err != nil {
		return fmt.Errorf("unable to list config files from %s: %v", oldConfigDir, err)
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		src := filepath.Join(oldConfigDir, "conf.d", f.Name())
		dst := filepath.Join(newConfigDir, "conf.d", f.Name())

		if f.Name() == "docker_daemon.yaml" {
			if convertDocker {
				err := legacy.ImportDockerConf(src, filepath.Join(newConfigDir, "conf.d", "docker.yaml"), force)
				if err != nil {
					return err
				}
			} else {
				fmt.Printf("ignoring %s, manualy use '--docker' option to convert it to the new format\n", src)
			}
			continue
		} else if f.Name() == "docker.yaml" {
			// if people upgrade from a very old version of the agent who ship the old docker check.
			fmt.Printf("ignoring %s, old docker check has been deprecated.\n", src)
			continue
		}

		if err := copyFile(src, dst, force); err != nil {
			return fmt.Errorf("unable to copy %s to %s: %v", src, dst, err)
		}

		fmt.Printf("Copied %s over the new conf.d directory\n", f.Name())
	}

	// move existing config templates to the new auto_conf directory
	autoConfFiles, err := ioutil.ReadDir(filepath.Join(oldConfigDir, "conf.d", "auto_conf"))
	if err != nil {
		return fmt.Errorf("unable to list auto_conf files from %s: %v", oldConfigDir, err)
	}

	for _, f := range autoConfFiles {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		src := filepath.Join(oldConfigDir, "conf.d", "auto_conf", f.Name())
		dst := filepath.Join(newConfigDir, "conf.d", "auto_conf", f.Name())

		if err := copyFile(src, dst, force); err != nil {
			fmt.Fprintf(os.Stderr, "unable to copy %s to %s: %v\n", src, dst, err)
			continue
		}

		fmt.Printf("Copied %s over the new auto_conf directory\n", f.Name())
	}

	// marshal the config object to YAML
	b, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		return fmt.Errorf("unable to unmarshal config to YAML: %v", err)
	}

	// dump the current configuration to datadog.yaml
	// file permissions will be used only to create the file if doesn't exist,
	// please note on Windows such permissions have no effect.
	if err = ioutil.WriteFile(datadogYamlPath, b, 0640); err != nil {
		return fmt.Errorf("unable to unmarshal config to %s: %v", datadogYamlPath, err)
	}

	fmt.Printf("Successfully imported the contents of %s into %s\n", datadogConfPath, datadogYamlPath)

	// Extract trace-agent specific info and dump it to its own config file.
	imported, err := configTraceAgent(datadogConfPath, traceAgentConfPath, force)
	if err != nil {
		return fmt.Errorf("failed to import Trace Agent specific settings: %v", err)
	}
	if imported {
		fmt.Printf("Wrote Trace Agent specific settings to %s\n", traceAgentConfPath)
	}

	// Extract process-agent specific info and dump it to its own config file.
	imported, err = configProcessAgent(datadogConfPath, processAgentConfPath, force)
	if err != nil {
		return fmt.Errorf("failed to import Process Agent specific settings: %v", err)
	}
	if imported {
		fmt.Printf("Wrote Process Agent specific settings to %s\n", processAgentConfPath)
	}

	return nil
}

// Copy the src file to dst. File attributes won't be copied.
func copyFile(src, dst string, overwrite bool) error {
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

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
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

// configProcessAgent extracts process-agent specific info and dump to its own config file
func configProcessAgent(datadogConfPath, processAgentConfPath string, overwrite bool) (bool, error) {
	// if the file exists check whether we can overwrite
	if _, err := os.Stat(processAgentConfPath); !os.IsNotExist(err) {
		if overwrite {
			// we'll overwrite, backup the original file first
			err = os.Rename(processAgentConfPath, processAgentConfPath+".bak")
			if err != nil {
				return false, fmt.Errorf("unable to create a backup for the existing file: %s", processAgentConfPath)
			}
		} else {
			return false, fmt.Errorf("destination file %s already exists, run the command again with --force or -f to overwrite it", processAgentConfPath)
		}
	}

	return legacy.ImportProcessAgentConfig(datadogConfPath, processAgentConfPath)
}
