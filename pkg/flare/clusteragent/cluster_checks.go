// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare"
)

// ClusterChecksOptions selects what GetClusterChecks prints
type ClusterChecksOptions struct {
	// CheckName only shows the configs of this check.
	CheckName string
	// Verbose prints the full check configurations instead of one line per instance.
	Verbose bool
	// JSON prints the raw dispatching state.
	JSON bool
}

// GetClusterChecks dumps the clustercheck dispatching state to the writer
func GetClusterChecks(w io.Writer, opts ClusterChecksOptions, c ipc.HTTPClient) error {
	urlstr := fmt.Sprintf("https://localhost:%v/api/v1/clusterchecks", pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"))
	urlValues := url.Values{}
	urlValues.Set("scrub", "true")

	if w != color.Output {
		color.NoColor = true
	}

	if !pkgconfigsetup.Datadog().GetBool("cluster_checks.enabled") {
		fmt.Fprintln(w, "Cluster-checks are not enabled")
		return nil
	}

	r, err := c.Get(urlstr, ipchttp.WithLeaveConnectionOpen, ipchttp.WithValues(urlValues))
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(w, "The agent ran into an error while checking config: %s\n", string(r))
		} else {
			fmt.Fprintf(w, "Failed to query the agent (running?): %s\n", err)
		}
		return err
	}

	var cr types.StateResponse
	err = json.Unmarshal(r, &cr)
	if err != nil {
		return err
	}

	if opts.JSON {
		out, err := json.MarshalIndent(cr, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(w, string(out))
		return nil
	}

	renderClusterChecks(w, cr, opts, time.Now())
	return nil
}

// renderClusterChecks writes the clustercheck dispatching state.
func renderClusterChecks(w io.Writer, cr types.StateResponse, opts ClusterChecksOptions, now time.Time) {
	// Gracefully exit when dispatcher is not running
	if len(cr.NotRunning) > 0 {
		fmt.Fprintf(w, "Cluster-check dispatching logic not running: %s\n", cr.NotRunning)
		return
	}

	// Print warmup message
	if cr.Warmup {
		fmt.Fprintf(w, "=== %s in progress ===\n", color.BlueString("Warmup"))
		fmt.Fprintln(w, "No configuration has been processed yet")
		fmt.Fprintln(w, "")
	}

	printDispatching(w, cr.Dispatching)
	printRunnerGroups(w, cr)
	printWarnings(w, cr, now)
	printUnassigned(w, cr, opts)

	// Print summary of agents
	if len(cr.Nodes) == 0 {
		fmt.Fprintf(w, "=== %s agent reporting ===\n", color.RedString("Zero"))
		fmt.Fprintln(w, "No check will be dispatched until agents report to the cluster-agent")
		return
	}
	fmt.Fprintf(w, "=== %d agent(s) reporting ===\n", len(cr.Nodes))
	sort.Slice(cr.Nodes, func(i, j int) bool {
		if cr.Nodes[i].Group != cr.Nodes[j].Group {
			return cr.Nodes[i].Group < cr.Nodes[j].Group
		}
		return cr.Nodes[i].Name < cr.Nodes[j].Name
	})
	table := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(table, "Name\tType\tGroup\tWorkers\tUtilization\tRunning checks\tLast seen")
	for _, n := range cr.Nodes {
		workers, utilization, lastSeen := "-", "-", "-"
		if n.Workers > 0 {
			workers = strconv.Itoa(n.Workers)
		}
		if n.Utilization != nil {
			utilization = fmt.Sprintf("%.1f%%", *n.Utilization*100)
		}
		if n.LastHeartbeat > 0 {
			lastSeen = formatAge(now.Sub(time.Unix(n.LastHeartbeat, 0)))
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n", n.Name, nodeTypeName(n.NodeType), groupName(n.Group), workers, utilization, len(n.Configs), lastSeen)
	}
	table.Flush()

	// Print per-node configurations with execution status, by runner group
	// when any is in use
	byGroup := cr.Dispatching != nil && len(cr.Dispatching.RunnerGroups) > 0
	for _, node := range cr.Nodes {
		byGroup = byGroup || node.Group != ""
	}
	lastGroup := ""
	for i, node := range cr.Nodes {
		if byGroup && (i == 0 || node.Group != lastGroup) {
			fmt.Fprintf(w, "\n=== Runner group %s ===\n", color.HiCyanString(groupName(node.Group)))
			lastGroup = node.Group
		}
		if len(node.Configs) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n===== Checks on %s =====\n", color.HiMagentaString(node.Name))
		if !opts.Verbose {
			printInstances(w, node, opts.CheckName, now)
			continue
		}
		for _, configResp := range node.Configs {
			flare.PrintClusterCheckConfig(w, configResp.Config, opts.CheckName, configResp.InstanceIDs)
			if len(node.Stats) > 0 {
				printCheckExecutionStatus(w, configResp.Config, node.Stats, opts.CheckName, configResp.InstanceIDs)
			}
		}
	}

	if !opts.Verbose {
		fmt.Fprintln(w, "\nUse --verbose to print the full check configurations.")
	}
}

// printUnassigned prints the dangling configs: one line per config with the
// reason it's waiting, or the full configs when verbose.
func printUnassigned(w io.Writer, cr types.StateResponse, opts ClusterChecksOptions) {
	var dangling []integration.Config
	for _, c := range cr.Dangling {
		if opts.CheckName == "" || c.Name == opts.CheckName {
			dangling = append(dangling, c)
		}
	}
	if len(dangling) == 0 {
		return
	}
	fmt.Fprintf(w, "=== %d %s configuration(s) ===\n", len(dangling), color.RedString("unassigned"))
	if opts.Verbose {
		for _, c := range dangling {
			if reason := cr.UnassignedReasons[c.Name]; reason != "" {
				fmt.Fprintf(w, "\n%s: %s", color.GreenString(c.Name), reason)
			}
			flare.PrintClusterCheckConfig(w, c, opts.CheckName, nil)
		}
		fmt.Fprintln(w, "")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(table, "Check\tInstances\tReason\tSource")
	for _, c := range dangling {
		fmt.Fprintf(table, "%s\t%d\t%s\t%s\n", c.Name, len(c.Instances), cr.UnassignedReasons[c.Name], c.Source)
	}
	table.Flush()
	fmt.Fprintln(w, "")
}

// printInstances prints one line per check instance of a node, with its
// execution status when the runner reports stats, then the failing
// instances' last errors.
func printInstances(w io.Writer, node types.StateNodeResponse, checkName string, now time.Time) {
	table := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	// Status is last: tabwriter counts color escape codes as width, which would misalign later columns.
	fmt.Fprintln(table, "Check\tInstance ID\tAvg exec\tLast run\tStatus")
	var errors []string
	for _, configResp := range node.Configs {
		c := configResp.Config
		if checkName != "" && c.Name != checkName {
			continue
		}
		ids := configResp.InstanceIDs
		if len(ids) != len(c.Instances) {
			ids = make([]string, len(c.Instances))
			for i, inst := range c.Instances {
				ids[i] = string(checkid.BuildID(c.Name, c.FastDigest(), inst, c.InitConfig))
			}
		}
		for _, id := range ids {
			status, avgExec, lastRun := "-", "-", "-"
			if s, found := node.Stats[id]; found {
				status = color.GreenString("OK")
				if s.LastExecFailed {
					status = color.RedString("ERROR")
					if s.LastError != "" {
						errors = append(errors, fmt.Sprintf("%s: %s", id, firstLine(s.LastError)))
					}
				}
				avgExec = (time.Duration(s.AverageExecutionTime) * time.Millisecond).String()
				if s.LastExecutionDate > 0 {
					lastRun = formatAge(now.Sub(time.UnixMilli(s.LastExecutionDate)))
				}
			}
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", c.Name, id, avgExec, lastRun, status)
		}
	}
	table.Flush()
	for _, e := range errors {
		fmt.Fprintf(w, "%s %s\n", color.RedString("Last error"), e)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	const maxLen = 200
	if len(line) > maxLen {
		line = line[:maxLen] + "..."
	}
	return line
}

// printDispatching prints the dispatching mode and settings.
func printDispatching(w io.Writer, d *types.DispatchingState) {
	if d == nil {
		return
	}
	fmt.Fprintln(w, "=== Cluster checks dispatching ===")
	if d.AdvancedDispatching {
		fmt.Fprintf(w, "Advanced dispatching: %s, rebalanced every %s\n", color.GreenString("enabled"), d.RebalancePeriod)
	} else {
		fmt.Fprintf(w, "Advanced dispatching: %s (%s)\n", color.YellowString("disabled"), d.AdvancedDispatchingReason)
	}
	fmt.Fprintf(w, "Excluded checks: %s\n", listOrNone(d.ExcludedChecks))
	fmt.Fprintf(w, "Checks excluded from rebalancing: %s\n", listOrNone(d.ExcludedChecksFromDispatching))
	fmt.Fprintln(w, "")
}

// printRunnerGroups prints the declared runner groups and their live workers.
func printRunnerGroups(w io.Writer, cr types.StateResponse) {
	if cr.Dispatching == nil || len(cr.Dispatching.RunnerGroups) == 0 {
		return
	}
	live := make(map[string]int)
	for _, n := range cr.Nodes {
		live[n.Group]++
	}
	fmt.Fprintln(w, "=== Runner groups ===")
	table := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(table, "Group\tClaimed checks\tLive workers")
	for _, g := range cr.Dispatching.RunnerGroups {
		count := strconv.Itoa(live[g.Name])
		if live[g.Name] == 0 {
			count = color.RedString("0")
		}
		fmt.Fprintf(table, "%s\t%s\t%s\n", g.Name, listOrNone(g.Checks), count)
	}
	fmt.Fprintf(table, "%s\t%s\t%d\n", groupName(""), "all other checks", live[""])
	table.Flush()
	fmt.Fprintln(w, "")
}

// printWarnings prints worker problems: unknown runner groups, missing runner
// stats, failing check instances and heartbeats close to expiring.
func printWarnings(w io.Writer, cr types.StateResponse, now time.Time) {
	declared := make(map[string]bool)
	advanced := false
	var expiration int64
	if cr.Dispatching != nil {
		for _, g := range cr.Dispatching.RunnerGroups {
			declared[g.Name] = true
		}
		advanced = cr.Dispatching.AdvancedDispatching
		expiration = cr.Dispatching.NodeExpirationSeconds
	}

	var warnings []string
	for _, n := range cr.Nodes {
		if n.Group != "" && !declared[n.Group] {
			warnings = append(warnings, fmt.Sprintf("%s declares unknown runner group %q: it receives no cluster check", n.Name, n.Group))
		}
		if advanced && n.NodeType == types.NodeTypeCLCRunner && len(n.Stats) == 0 && len(n.Configs) > 0 {
			warnings = append(warnings, n.Name+": no check stats collected yet, or the runner API is unreachable")
		}
		failing := 0
		for _, s := range n.Stats {
			if s.IsClusterCheck && s.LastExecFailed {
				failing++
			}
		}
		if failing > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: %d check instance(s) failed their last run", n.Name, failing))
		}
		if expiration > 0 && n.LastHeartbeat > 0 && now.Sub(time.Unix(n.LastHeartbeat, 0)) > time.Duration(expiration)*time.Second/2 {
			warnings = append(warnings, fmt.Sprintf("%s: last seen %s, expires after %ds without a heartbeat", n.Name, formatAge(now.Sub(time.Unix(n.LastHeartbeat, 0))), expiration))
		}
	}
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(w, "=== %s ===\n", color.YellowString("Warnings"))
	for _, warning := range warnings {
		fmt.Fprintf(w, "- %s\n", warning)
	}
	fmt.Fprintln(w, "")
}

func nodeTypeName(t types.NodeType) string {
	switch t {
	case types.NodeTypeCLCRunner:
		return "clc-runner"
	case types.NodeTypeNodeAgent:
		return "node-agent"
	default:
		return "unknown"
	}
}

func groupName(group string) string {
	if group == "" {
		return "general"
	}
	return group
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return d.Round(time.Second).String() + " ago"
}

// printCheckExecutionStatus prints the execution status for each instance of a config,
// matching the node agent `agent status collector` output format.
func printCheckExecutionStatus(w io.Writer, c integration.Config, stats types.CLCRunnersStats, checkName string, instanceIDs []string) {
	if checkName != "" && c.Name != checkName {
		return
	}
	if len(stats) == 0 {
		return
	}

	for _, id := range instanceIDs {
		s, found := stats[id]
		if !found {
			continue
		}

		statusStr := color.GreenString("OK")
		if s.LastExecFailed {
			statusStr = color.RedString("ERROR")
		}
		fmt.Fprintf(w, "  Instance ID: %s [%s]\n", id, statusStr)
		fmt.Fprintf(w, "  Total Runs: %d\n", s.TotalRuns)
		fmt.Fprintf(w, "  Metric Samples: Last Run: %d, Total: %d\n", s.MetricSamples, s.TotalMetricSamples)
		fmt.Fprintf(w, "  Events: Last Run: %d, Total: %d\n", s.Events, s.TotalEvents)
		fmt.Fprintf(w, "  Service Checks: Last Run: %d, Total: %d\n", s.ServiceChecks, s.TotalServiceChecks)
		fmt.Fprintf(w, "  Average Execution Time : %s\n", (time.Duration(s.AverageExecutionTime) * time.Millisecond).String())
		if s.LastExecutionDate > 0 {
			fmt.Fprintf(w, "  Last Execution Date : %s\n", time.UnixMilli(s.LastExecutionDate).UTC().Format("2006-01-02 15:04:05 MST"))
		}
		if s.LastSuccessDate > 0 {
			fmt.Fprintf(w, "  Last Successful Execution Date : %s\n", time.Unix(s.LastSuccessDate, 0).UTC().Format("2006-01-02 15:04:05 MST"))
		}
		if s.LastError != "" {
			fmt.Fprintf(w, "  %s: %s\n", color.RedString("Last Error"), s.LastError)
		}
		fmt.Fprintln(w, "")
	}
}

// GetEndpointsChecks dumps the endpointschecks dispatching state to the writer
func GetEndpointsChecks(w io.Writer, checkName string, c ipc.HTTPClient) error {
	if !endpointschecksEnabled() {
		return nil
	}

	urlstr := fmt.Sprintf("https://localhost:%v/api/v1/endpointschecks/configs", pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"))

	if w != color.Output {
		color.NoColor = true
	}

	// Query the cluster agent API
	r, err := c.Get(urlstr, ipchttp.WithLeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(w, "The agent ran into an error while checking config: %s\n", string(r))
		} else {
			fmt.Fprintf(w, "Failed to query the agent (running?): %s\n", err)
		}
		return err
	}

	var cr types.ConfigResponse
	if err = json.Unmarshal(r, &cr); err != nil {
		return err
	}

	// Print summary of pod-backed endpointschecks
	fmt.Fprintf(w, "\n===== %d Pod-backed Endpoints-Checks scheduled =====\n", len(cr.Configs))
	for _, c := range cr.Configs {
		flare.PrintClusterCheckConfig(w, c, checkName, nil)
	}

	return nil
}

func endpointschecksEnabled() bool {
	return slices.Contains(pkgconfigsetup.Datadog().GetStringSlice("extra_config_providers"), names.KubeEndpointsRegisterName)
}
