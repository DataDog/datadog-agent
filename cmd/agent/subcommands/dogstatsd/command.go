// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

// Package dogstatsd contains "agent dogstatsd" subcommands
package dogstatsd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/DataDog/zstd"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	cconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type topFlags struct {
	path     string
	nmetrics int
	ntags    int
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(topContexts,
				fx.Supply(&topFlags),
				fx.Supply(core.BundleParams{
					ConfigParams: cconfig.NewAgentParams(globalParams.ConfFilePath, cconfig.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    logimpl.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
			)
		},
	}
	topCmd.Flags().StringVarP(&topFlags.path, "path", "p", "", "use specified file for input instead of getting contexts from the agent")
	topCmd.Flags().IntVarP(&topFlags.nmetrics, "num-metrics", "m", 10, "number of metrics to show")
	topCmd.Flags().IntVarP(&topFlags.ntags, "mum-tags", "t", 5, "number of tags to show per metric")

	c.AddCommand(topCmd)

	c.AddCommand(&cobra.Command{
		Use:   "dump-contexts",
		Short: "Write currently tracked contexts as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(dumpContexts,
				fx.Supply(core.BundleParams{
					ConfigParams: cconfig.NewAgentParams(globalParams.ConfFilePath, cconfig.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    logimpl.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
			)
		},
	})

	return []*cobra.Command{c}
}

func triggerDump(config cconfig.Component) (string, error) {
	c := util.GetClient(false)
	addr, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return "", err
	}

	port := config.GetInt("cmd_port")
	url := fmt.Sprintf("https://%v:%v/agent/dogstatsd-contexts-dump", addr, port)

	err = util.SetAuthToken(config)
	if err != nil {
		return "", err
	}

	body, err := util.DoPost(c, url, "", nil)
	if err != nil {
		return "", err
	}

	var path string
	if err = json.Unmarshal(body, &path); err != nil {
		return "", err
	}

	return path, nil
}

func dumpContexts(config cconfig.Component, _ log.Component) error {
	path, err := triggerDump(config)
	if err != nil {
		return err
	}

	fmt.Printf("Wrote %s\n", path)

	return nil
}

type metric struct {
	count uint
	tags  map[string]struct{}
}

func topContexts(config cconfig.Component, flags *topFlags, _ log.Component) error {
	var err error

	path := flags.path
	if path == "" {
		path, err = triggerDump(config)
		if err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = bufio.NewReader(f)

	if strings.HasSuffix(path, ".zstd") {
		d := zstd.NewReader(r)
		defer d.Close()
		r = d
	}

	dec := json.NewDecoder(r)

	repr := aggregator.ContextDebugRepr{}

	metrics := make(map[string]*metric)

	for {
		err := dec.Decode(&repr)
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		m := metrics[repr.Name]
		if m == nil {
			m = &metric{
				tags: make(map[string]struct{}, len(repr.MetricTags)),
			}
			metrics[repr.Name] = m
		}

		m.count++

		for _, tag := range repr.MetricTags {
			m.tags[tag] = struct{}{}
		}
	}

	fmt.Printf(" % 10s\t%s\t(%s)\n", "Contexts", "Metric name", "number of unique values for each tag")

	ks := make([]string, 0, len(metrics))
	for k := range metrics {
		ks = append(ks, k)
	}

	sort.Slice(ks, func(i, j int) bool {
		n := metrics[ks[i]].count
		m := metrics[ks[j]].count
		if n == m {
			return ks[i] < ks[j]
		}
		return n > m
	})

	top := ks
	rest := []string{}
	limit := flags.nmetrics
	// +1 to avoid showing "1 more", just show it.
	if len(ks) > limit+1 {
		top = ks[:limit]
		rest = ks[limit:]
	}

	for _, k := range top {
		m := metrics[k]

		fmt.Printf(" % 10d\t%s\t(", m.count, k)
		printTopTags(m, flags.ntags)
		fmt.Println(")")
	}

	if len(rest) > 0 {
		var sum uint
		for _, k := range rest {
			sum += metrics[k].count
		}
		fmt.Printf(" % 10d\t(other %d metrics)\n", sum, len(rest))
	}

	return nil
}

func printTopTags(m *metric, limit int) {
	ts := make(map[string]uint)
	for tag := range m.tags {
		k, _, _ := strings.Cut(tag, ":")
		ts[k]++
	}

	ks := make([]string, 0, len(ts))
	for k := range ts {
		ks = append(ks, k)
	}

	sort.Slice(ks, func(i, j int) bool {
		n := ts[ks[i]]
		m := ts[ks[j]]
		if n == m {
			return ks[i] < ks[j]
		}
		return n > m
	})

	top := ks
	rest := []string{}

	// +1 to avoid showing "1 more", just show it.
	if len(ks) > limit+1 {
		top = ks[:limit]
		rest = ks[limit:]
	}

	for i, k := range top {
		if i > 0 {
			fmt.Printf(", ")
		}

		fmt.Printf("%d %s", ts[k], k)
	}

	if len(rest) > 0 {
		var sum uint
		for _, k := range rest {
			sum += ts[k]
		}

		if len(top) > 0 {
			fmt.Printf(", ")
		}

		fmt.Printf("%d values in %d other tags", sum, len(rest))
	}
}
