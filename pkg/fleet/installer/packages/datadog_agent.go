// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/datadogagent"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentPackage = "datadog-agent"

	agentSymlink       = "/usr/bin/datadog-agent"
	installerSymlink   = "/usr/bin/datadog-installer"
	legacyAgentSymlink = "/opt/datadog-agent"

	stablePath     = "/opt/datadog-packages/datadog-agent/stable"
	experimentPath = "/opt/datadog-packages/datadog-agent/experiment"

	agentUnit          = "datadog-agent.service"
	installerAgentUnit = "datadog-agent-installer.service"
	traceAgentUnit     = "datadog-agent-trace.service"
	processAgentUnit   = "datadog-agent-process.service"
	systemProbeUnit    = "datadog-agent-sysprobe.service"
	securityAgentUnit  = "datadog-agent-security.service"
	agentExp           = "datadog-agent-exp.service"
	installerAgentExp  = "datadog-agent-installer-exp.service"
	traceAgentExp      = "datadog-agent-trace-exp.service"
	processAgentExp    = "datadog-agent-process-exp.service"
	systemProbeExp     = "datadog-agent-sysprobe-exp.service"
	securityAgentExp   = "datadog-agent-security-exp.service"

	installerCaller = "installer"
)

var (
	stableUnits = []string{
		agentUnit,
		installerAgentUnit,
		traceAgentUnit,
		processAgentUnit,
		systemProbeUnit,
		securityAgentUnit,
	}
	experimentalUnits = []string{
		agentExp,
		installerAgentExp,
		traceAgentExp,
		processAgentExp,
		systemProbeExp,
		securityAgentExp,
	}
)

// PrepareAgent prepares the machine to install the agent
func PrepareAgent(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "prepare_agent")
	defer func() { span.Finish(err) }()

	for _, unit := range stableUnits {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
		if err := systemd.DisableUnit(ctx, unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
	}
	return packagemanager.RemovePackage(ctx, agentPackage)
}

// SetupAgent installs and starts the agent
func SetupAgent(ctx context.Context, _ []string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent, reverting: %s", err)
			err = errors.Join(err, RemoveAgent(ctx))
		}
		span.Finish(err)
	}()

	err = datadogagent.PostInstall(ctx, stablePath, installerCaller)
	if err != nil {
		return err
	}

	err = setupStableUnits(ctx)
	return err
}

// PreRemoveAgent performs pre-removal steps for the agent
// All operations are allowed to fail
func PreRemoveAgent(ctx context.Context, installPath string, caller string, upgrade bool) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "pre_remove_agent")
	defer func() {
		span.Finish(nil)
	}()

	// 1. Run pre.py for integration persistence
	if upgrade {
		if _, err := os.Stat(filepath.Join(installPath, "embedded/bin/python")); err == nil {
			cmd := exec.CommandContext(ctx, filepath.Join(installPath, "embedded/bin/python"), filepath.Join(installPath, "python-scripts/pre.py"), installPath)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("failed to run integration persistence in pre.py: %s\n", err.Error())
			}
		}
	}

	if !upgrade || caller != installerCaller { // We don't want to remove anything during installer remote upgrades as we may need to fall back
		// 2. Remove custom integrations
		if _, err := os.Stat(filepath.Join(installPath, "embedded/.installed_by_pkg.txt")); err == nil {
			fmt.Println("Removing integrations installed with the 'agent integration' command")
			if err := removeCustomIntegrations(ctx, installPath); err != nil {
				fmt.Printf("failed to remove custom integrations: %s\n", err.Error())
			}
		}

		// 3. Delete all the .pyc files. This MUST be done after using pip or any python, because executing python might generate .pyc files
		removeCompiledPythonFiles(installPath)
	}

	if !upgrade {
		// 4. Remove run dir
		os.RemoveAll(filepath.Join(installPath, "run"))
		// 5. Remove FIPS module
		os.Remove(filepath.Join(installPath, "embedded", "ssl", "fipsmodule.cnf"))
		// 6. Remove any file related to reinstalling non-core integrations (see python-scripts/packages.py for the names)
		os.Remove(filepath.Join(installPath, ".pre_python_installed_packages.txt"))
		os.Remove(filepath.Join(installPath, ".post_python_installed_packages.txt"))
		os.Remove(filepath.Join(installPath, ".diff_python_installed_packages.txt"))
		// 7. Remove install info
		installinfo.RemoveInstallInfo()
		// 8. Remove symlinks
		os.Remove(agentSymlink)
		os.Remove(legacyAgentSymlink)
		if target, err := os.Readlink(installerSymlink); err == nil && strings.HasPrefix(target, installPath) {
			os.Remove(installerSymlink)
		}
	}
}

// RemoveAgent stops and removes the agent
func RemoveAgent(ctx context.Context) error {
	span, ctx := telemetry.StartSpanFromContext(ctx, "remove_agent_units")
	var spanErr error
	defer func() { span.Finish(spanErr) }()
	// stop, disable, & delete units from disk
	spanErr = removeAgentUnits(ctx, agentExp, true)
	if spanErr != nil {
		log.Warnf("Failed to remove experimental units: %s", spanErr)
	}
	spanErr = removeAgentUnits(ctx, agentUnit, false)
	if spanErr != nil {
		log.Warnf("Failed to remove stable units: %s", spanErr)
	}

	PreRemoveAgent(ctx, stablePath, installerCaller, false)
	return nil
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) error {
	if err := datadogagent.PostInstall(ctx, experimentPath, installerCaller); err != nil {
		return err
	}
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	err := setupExperimentUnits(ctx)
	return err
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment(ctx context.Context) error {
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	if err := setupStableUnits(ctx); err != nil {
		return err
	}
	return removeAgentUnits(ctx, agentExp, true)
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(ctx context.Context) error {
	if err := setupStableUnits(ctx); err != nil {
		return err
	}
	return removeAgentUnits(ctx, agentExp, true)
}

func setupStableUnits(ctx context.Context) error {
	return setupAgentUnits(ctx, agentUnit, stableUnits)
}

func setupExperimentUnits(ctx context.Context) error {
	return setupAgentUnits(ctx, agentExp, experimentalUnits)
}

func removeAgentUnits(ctx context.Context, coreAgentUnit string, experiment bool) error {
	units, err := systemd.ListOnDiskAgentUnits(experiment)
	if err != nil {
		return fmt.Errorf("failed to list agent units: %v", err)
	}

	for _, unit := range units {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			return err
		}
	}

	if err := systemd.DisableUnit(ctx, coreAgentUnit); err != nil {
		return err
	}

	for _, unit := range units {
		if err := systemd.RemoveUnit(ctx, unit); err != nil {
			return err
		}
	}
	return nil
}

func setupAgentUnits(ctx context.Context, coreAgentUnit string, units []string) error {
	for _, unit := range units {
		if err := systemd.WriteEmbeddedUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to load %s: %v", unit, err)
		}
	}

	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	// enabling the core agent unit only is enough as others are triggered by it
	if err := systemd.EnableUnit(ctx, coreAgentUnit); err != nil {
		return fmt.Errorf("failed to enable %s: %v", coreAgentUnit, err)
	}

	_, err := os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	} else if os.IsNotExist(err) {
		// this is expected during a fresh install with the install script / ansible / chef / etc...
		// the config is populated afterwards by the install method and the agent is restarted
		return nil
	}
	if err = systemd.StartUnit(ctx, coreAgentUnit); err != nil {
		return err
	}
	return nil
}

// removeCustomIntegrations removes custom integrations that are not installed by the package
//
// Since 6.18.0, a file containing all integrations files which have been installed by
// the package is available. We use it to remove only the datadog-related check files which
// have *NOT* been installed by the package (eg: installed using the `integration` command).
func removeCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_custom_integrations")
	defer func() { span.Finish(err) }()

	// Use an in-memory map to store all integration paths
	allIntegrations, err := filepath.Glob(installPath + "/embedded/lib/python*/site-packages/datadog_*")
	if err != nil {
		return err
	}

	// Read the list of installed files
	installedByPkg, err := os.ReadFile(filepath.Join(installPath, "embedded", ".installed_by_pkg.txt"))
	if err != nil {
		return err
	}

	// Create a set of paths installed by the package
	installedByPkgSet := make(map[string]struct{})
	for _, line := range strings.Split(string(installedByPkg), "\n") {
		if line != "" {
			installedByPkgSet[line] = struct{}{}
		}
	}

	// Remove paths that are in allIntegrations but not in installedByPkgSet
	for _, path := range allIntegrations {
		if _, exists := installedByPkgSet[path]; !exists {
			// Remove the directory if it was not installed by the package.
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}

	return nil
}

// removeCompiledPythonFiles removes compiled Python files (.pyc, .pyo) and __pycache__ directories
func removeCompiledPythonFiles(installPath string) {
	// Remove files in in "{installPath}/embedded/.py_compiled_files.txt"
	if _, err := os.Stat(filepath.Join(installPath, "embedded/.py_compiled_files.txt")); err == nil {
		compiledFiles, err := os.ReadFile(filepath.Join(installPath, "embedded/.py_compiled_files.txt"))
		if err != nil {
			fmt.Printf("failed to read compiled files list: %s\n", err.Error())
		} else {
			for _, file := range strings.Split(string(compiledFiles), "\n") {
				if strings.HasPrefix(file, installPath) {
					if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
						fmt.Printf("failed to remove compiled file %s: %s\n", file, err.Error())
					}
				}
			}
		}
	}
	// Remove files in {installPath}/bin/agent/dist
	err := filepath.Walk(filepath.Join(installPath, "bin", "agent", "dist"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(info.Name(), ".pyc") || strings.HasSuffix(info.Name(), ".pyo") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("failed to remove compiled files: %s\n", err.Error())
	}
	// Remove files in {installPath}/python-scripts
	err = filepath.Walk(filepath.Join(installPath, "python-scripts"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(info.Name(), ".pyc") || strings.HasSuffix(info.Name(), ".pyo") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("failed to remove compiled files: %s\n", err.Error())
	}
}
