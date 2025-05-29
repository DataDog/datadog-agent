// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package daemon provides the installer daemon commands.
package daemon

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/updater/localapiclient"
	"github.com/DataDog/datadog-agent/comp/updater/localapiclient/localapiclientimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	command.GlobalParams
	pkg     string
	version string
	catalog string
	configs string
}

func apiCommands(global *command.GlobalParams) []*cobra.Command {
	setCatalogCmd := &cobra.Command{
		Hidden: true,
		Use:    "set-catalog catalog",
		Short:  "Internal command to set the catalog to use",
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(catalog, &cliParams{
				GlobalParams: *global,
				catalog:      args[0],
			})
		},
	}

	setConfigCatalogCmd := &cobra.Command{
		Hidden: true,
		Use:    "set-config-catalog configs",
		Short:  "Internal command to set the config catalog to use",
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(setConfigCatalog, &cliParams{
				GlobalParams: *global,
				configs:      args[0],
			})
		},
	}

	installCmd := &cobra.Command{
		Use:     "install package version",
		Aliases: []string{"install"},
		Short:   "Installs a package to the expected version",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(install, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
				version:      args[1],
			})
		},
	}
	removeCmd := &cobra.Command{
		Use:   "remove package",
		Short: "Removes a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(remove, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
			})
		},
	}
	startExperimentCmd := &cobra.Command{
		Use:     "start-experiment package version",
		Aliases: []string{"start"},
		Short:   "Starts an experiment",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(start, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
				version:      args[1],
			})
		},
	}
	stopExperimentCmd := &cobra.Command{
		Use:     "stop-experiment package",
		Aliases: []string{"stop"},
		Short:   "Stops an experiment",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(stop, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
			})
		},
	}
	promoteExperimentCmd := &cobra.Command{
		Use:     "promote-experiment package",
		Aliases: []string{"promote"},
		Short:   "Promotes an experiment",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(promote, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
			})
		},
	}
	startConfigExperimentCmd := &cobra.Command{
		Use:     "start-config-experiment package version",
		Aliases: []string{"start-config"},
		Short:   "Starts an experiment",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(startConfig, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
				version:      args[1],
			})
		},
	}
	stopConfigExperimentCmd := &cobra.Command{
		Use:     "stop-config-experiment package",
		Aliases: []string{"stop-config"},
		Short:   "Stops an experiment",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(stopConfig, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
			})
		},
	}
	promoteConfigExperimentCmd := &cobra.Command{
		Use:     "promote-config-experiment package",
		Aliases: []string{"promote-config"},
		Short:   "Promotes an experiment",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return experimentFxWrapper(promoteConfig, &cliParams{
				GlobalParams: *global,
				pkg:          args[0],
			})
		},
	}
	remoteConfigStatusCmd := &cobra.Command{
		Hidden: true,
		Use:    "rc-status",
		Short:  "Internal command to print the installer Remote Config status as a JSON",
		RunE: func(_ *cobra.Command, _ []string) error {
			return experimentFxWrapper(status, &cliParams{
				GlobalParams: *global,
			})
		},
	}
	return []*cobra.Command{
		setCatalogCmd,
		setConfigCatalogCmd,
		startExperimentCmd,
		stopExperimentCmd,
		promoteExperimentCmd,
		installCmd,
		removeCmd,
		startConfigExperimentCmd,
		stopConfigExperimentCmd,
		promoteConfigExperimentCmd,
		remoteConfigStatusCmd,
	}
}

func experimentFxWrapper(f interface{}, params *cliParams) error {
	return fxutil.OneShot(f,
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            log.ForOneShot("INSTALLER", "off", true),
		}),
		core.Bundle(),
		fx.Supply(params),
		localapiclientimpl.Module(),
	)
}

func catalog(params *cliParams, client localapiclient.Component) error {
	err := client.SetCatalog(params.catalog)
	if err != nil {
		fmt.Println("Error setting catalog:", err)
		return err
	}
	return nil
}

func setConfigCatalog(params *cliParams, client localapiclient.Component) error {
	err := client.SetConfigCatalog(params.configs)
	if err != nil {
		fmt.Println("Error setting config catalog:", err)
		return err
	}
	return nil
}

func start(params *cliParams, client localapiclient.Component) error {
	err := client.StartExperiment(params.pkg, params.version)
	if err != nil {
		fmt.Println("Error starting experiment:", err)
		return err
	}
	return nil
}

func stop(params *cliParams, client localapiclient.Component) error {
	err := client.StopExperiment(params.pkg)
	if err != nil {
		fmt.Println("Error stopping experiment:", err)
		return err
	}
	return nil
}

func promote(params *cliParams, client localapiclient.Component) error {
	err := client.PromoteExperiment(params.pkg)
	if err != nil {
		fmt.Println("Error promoting experiment:", err)
		return err
	}
	return nil
}

func startConfig(params *cliParams, client localapiclient.Component) error {
	err := client.StartConfigExperiment(params.pkg, params.version)
	if err != nil {
		fmt.Println("Error starting config experiment:", err)
		return err
	}
	return nil
}

func stopConfig(params *cliParams, client localapiclient.Component) error {
	err := client.StopConfigExperiment(params.pkg)
	if err != nil {
		fmt.Println("Error stopping config experiment:", err)
		return err
	}
	return nil
}

func promoteConfig(params *cliParams, client localapiclient.Component) error {
	err := client.PromoteConfigExperiment(params.pkg)
	if err != nil {
		fmt.Println("Error promoting config experiment:", err)
		return err
	}
	return nil
}

func install(params *cliParams, client localapiclient.Component) error {
	err := client.Install(params.pkg, params.version)
	if err != nil {
		fmt.Println("Error installing package:", err)
		return err
	}
	return nil
}

func remove(params *cliParams, client localapiclient.Component) error {
	err := client.Remove(params.pkg)
	if err != nil {
		fmt.Println("Error removing package:", err)
		return err
	}
	return nil
}
func status(_ *cliParams, client localapiclient.Component) error {
	status, err := client.Status()
	if err != nil {
		fmt.Println("Error getting status:", err)
		return err
	}
	bytes, err := json.Marshal(status)
	if err != nil {
		fmt.Println("Error marshalling status:", err)
	}
	fmt.Fprintf(os.Stdout, "%s", bytes)
	return nil
}
