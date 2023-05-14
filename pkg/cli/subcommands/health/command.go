// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package health builds a 'health' command to be used in binaries.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	timeout int
}

type GlobalParams struct {
	ConfFilePath string
	ConfigName   string
	LoggerName   string
}

// MakeCommand returns a `health` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}
	cmd := &cobra.Command{
		Use:          "health",
		Short:        "Print the current agent health",
		Long:         ``,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()
			return fxutil.OneShot(requestHealth,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath, config.WithConfigName(globalParams.ConfigName)),
					LogParams:    log.LogForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle,
			)
		},
	}

	cmd.Flags().IntVarP(&cliParams.timeout, "timeout", "t", 20, "timeout in second to query the Agent")
	return cmd
}

func requestHealth(log log.Component, config config.Component, cliParams *cliParams) error {
	c := util.GetClient(false) // FIX: get certificates right then make this true

	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	var urlstr string
	if flavor.GetFlavor() == flavor.ClusterAgent {
		urlstr = fmt.Sprintf("https://%v:%v/status/health", ipcAddress, pkgconfig.Datadog.GetInt("cluster_agent.cmd_port"))
	} else {
		urlstr = fmt.Sprintf("https://%v:%v/agent/status/health", ipcAddress, pkgconfig.Datadog.GetInt("cmd_port"))
	}

	// Set session token
	err = util.SetAuthToken()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cliParams.timeout)*time.Second)
	defer cancel()

	r, err := util.DoGetWithContext(ctx, c, urlstr, util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}

		return fmt.Errorf("Could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues.", err)
	}

	s := new(health.Status)
	if err = json.Unmarshal(r, s); err != nil {
		return fmt.Errorf("Error unmarshalling json: %s", err)
	}

	sort.Strings(s.Unhealthy)
	sort.Strings(s.Healthy)

	statusString := color.GreenString("PASS")
	if len(s.Unhealthy) > 0 {
		statusString = color.RedString("FAIL")
	}
	fmt.Fprintln(color.Output, fmt.Sprintf("Agent health: %s", statusString))

	if len(s.Healthy) > 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s healthy components ===", color.GreenString(strconv.Itoa(len(s.Healthy)))))
		fmt.Fprintln(color.Output, strings.Join(s.Healthy, ", "))
	}
	if len(s.Unhealthy) > 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s unhealthy components ===", color.RedString(strconv.Itoa(len(s.Unhealthy)))))
		fmt.Fprintln(color.Output, strings.Join(s.Unhealthy, ", "))
		return fmt.Errorf("found %d unhealthy components", len(s.Unhealthy))
	}

	return nil
}
