// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmp implements the 'agent snmp' subcommand.
package snmp

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// argsType is an alias so we can inject the args via fx.
type argsType []string

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	snmpCmd := &cobra.Command{
		Use:   "ha-agent",
		Short: "High Availability Agent",
		Long:  ``,
	}

	snmpWalkCmd := &cobra.Command{
		Use:   "role <primary|secondary>",
		Short: "Set role.",
		Long:  `Set role.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(setRole,
				fx.Provide(func() argsType { return args }),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
			)
		},
	}

	snmpCmd.AddCommand(snmpWalkCmd)

	return []*cobra.Command{snmpCmd}
}

// setRole set HA agent role
func setRole(args argsType, config config.Component, logger log.Component) error {
	logger.Warnf("[HA Agent] args: %+v", args) // TODO: REMOVE ME
	if len(args) != 1 {
		return fmt.Errorf("only one argument is expected. %d arguments were given", len(args))
	}
	role := args[0]

	// Global Agent configuration
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	e := util.SetAuthToken(config)
	if e != nil {
		return e
	}
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/role/%s", ipcAddress, config.GetInt("cmd_port"), role)

	fmt.Printf("URL %s\n", urlstr)

	_, e = util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	if e != nil {
		return fmt.Errorf("Error stopping the agent: %v", e)
	}

	fmt.Printf("Successfully change role to %s\n", role)
	return nil
}
