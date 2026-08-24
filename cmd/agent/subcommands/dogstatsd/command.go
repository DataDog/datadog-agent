// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

// Package dogstatsd contains "agent dogstatsd" subcommands
package dogstatsd

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/dogstatsdcommon"
	"github.com/DataDog/datadog-agent/comp/core"
	cconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/contexttop"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type topFlags struct {
	path               string
	nmetrics           int
	ntags              int
	logLevelDefaultOff command.LogLevelDefaultOff
}

// Commands initializes dogstatsd sub-command tree.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	c := &cobra.Command{
		Use:   "dogstatsd",
		Short: "Inspect dogstatsd pipeline status",
	}

	topFlags := topFlags{}

	topCmd := &cobra.Command{
		Use:   "top",
		Short: "Display metrics with most contexts in the aggregator",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(topContexts,
				fx.Supply(&topFlags),
				fx.Supply(core.BundleParams{
					ConfigParams: cconfig.NewAgentParams(globalParams.ConfFilePath, cconfig.WithExtraConfFiles(globalParams.ExtraConfFilePath), cconfig.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, topFlags.logLevelDefaultOff.Value(), true)}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	topFlags.logLevelDefaultOff.Register(topCmd)
	topCmd.Flags().StringVarP(&topFlags.path, "path", "p", "", "use specified file for input instead of getting contexts from the agent")
	topCmd.Flags().IntVarP(&topFlags.nmetrics, "num-metrics", "m", 10, "number of metrics to show")
	topCmd.Flags().IntVarP(&topFlags.ntags, "mum-tags", "t", 5, "number of tags to show per metric")

	c.AddCommand(topCmd)

	c.AddCommand(&cobra.Command{
		Use:   "dump-contexts",
		Short: "Write currently tracked contexts as JSON",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(dumpContexts,
				fx.Supply(core.BundleParams{
					ConfigParams: cconfig.NewAgentParams(globalParams.ConfFilePath, cconfig.WithExtraConfFiles(globalParams.ExtraConfFilePath), cconfig.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, topFlags.logLevelDefaultOff.Value(), true)}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	})

	return []*cobra.Command{c}
}

func triggerDump(config cconfig.Component, client ipc.HTTPClient) (string, error) {
	addr, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}

	port := config.GetInt("cmd_port")
	url := fmt.Sprintf("https://%s/agent/dogstatsd-contexts-dump", net.JoinHostPort(addr, strconv.Itoa(port)))

	body, err := client.Post(url, "", nil)
	if err != nil {
		return "", err
	}

	var path string
	if err = json.Unmarshal(body, &path); err != nil {
		return "", err
	}

	return path, nil
}

func dumpContexts(config cconfig.Component, _ log.Component, client ipc.HTTPClient) error {
	if err := dogstatsdcommon.CheckDataPlaneOwnsDogstatsd(config); err != nil {
		return err
	}

	path, err := triggerDump(config, client)
	if err != nil {
		return err
	}

	fmt.Printf("Wrote %s\n", path)

	return nil
}

func topContexts(config cconfig.Component, flags *topFlags, _ log.Component, client ipc.HTTPClient) error {
	var err error

	path := flags.path
	if path == "" {
		if err := dogstatsdcommon.CheckDataPlaneOwnsDogstatsd(config); err != nil {
			return err
		}

		path, err = triggerDump(config, client)
		if err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", path)
	}

	top, err := contexttop.FromFile(path, flags.nmetrics, flags.ntags)
	if err != nil {
		return err
	}

	fmt.Printf(" % 10s\t%s\t(%s)\n", "Contexts", "Metric name", "number of unique values for each tag")
	for _, metric := range top.Metrics {
		fmt.Printf(" % 10d\t%s\t(", metric.Contexts, metric.Name)
		printTopTags(metric)
		fmt.Println(")")
	}

	if top.OtherMetrics > 0 {
		fmt.Printf(" % 10d\t(other %d metrics)\n", top.OtherContexts, top.OtherMetrics)
	}

	return nil
}

func printTopTags(metric contexttop.Metric) {
	for i, tag := range metric.Tags {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%d %s", tag.UniqueValues, tag.Key)
	}

	if metric.OtherTags > 0 {
		if len(metric.Tags) > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%d values in %d other tags", metric.OtherTagValues, metric.OtherTags)
	}
}
