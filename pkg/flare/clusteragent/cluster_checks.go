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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare"
)

// GetClusterChecks dumps the clustercheck dispatching state to the writer
func GetClusterChecks(w io.Writer, checkName string, c ipc.HTTPClient) error {
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

	renderClusterChecks(w, cr, checkName, time.Now())
	return nil
}

// renderClusterChecks writes the clustercheck dispatching state.
func renderClusterChecks(w io.Writer, cr types.StateResponse, checkName string, now time.Time) {
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

	// Print dangling configs
	if len(cr.Dangling) > 0 {
		fmt.Fprintf(w, "=== %d %s configurations ===\n", len(cr.Dangling), color.RedString("unassigned"))
		for _, c := range cr.Dangling {
			if checkName != "" && c.Name != checkName {
				continue
			}
			if reason := cr.UnassignedReasons[c.Name]; reason != "" {
				fmt.Fprintf(w, "\n%s: %s", color.GreenString(c.Name), reason)
			}
			flare.PrintClusterCheckConfig(w, c, checkName, nil)
		}
		fmt.Fprintln(w, "")
	}

	// Print summary of agents
	if len(cr.Nodes) == 0 {
		fmt.Fprintf(w, "=== %s agent reporting ===\n", color.RedString("Zero"))
		fmt.Fprintln(w, "No check will be dispatched until agents report to the cluster-agent")
		return
	}
	fmt.Fprintf(w, "=== %d agents reporting ===\n", len(cr.Nodes))
	sort.Slice(cr.Nodes, func(i, j int) bool {
		if cr.Nodes[i].Group != cr.Nodes[j].Group {
			return cr.Nodes[i].Group < cr.Nodes[j].Group
		}
		return cr.Nodes[i].Name < cr.Nodes[j].Name
	})
	table := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(table, "\nName\tType\tGroup\tWorkers\tUtilization\tRunning checks\tLast seen")
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

	// Print per-node configurations with execution status
	for _, node := range cr.Nodes {
		if len(node.Configs) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n===== Checks on %s =====\n", color.HiMagentaString(node.Name))
		for _, configResp := range node.Configs {
			flare.PrintClusterCheckConfig(w, configResp.Config, checkName, configResp.InstanceIDs)
			if len(node.Stats) > 0 {
				printCheckExecutionStatus(w, configResp.Config, node.Stats, checkName, configResp.InstanceIDs)
			}
		}
	}
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

// backupHeartbeatMargin mirrors the cluster checks provider: a runner whose
// main loop is busy only sends its backup heartbeat this long before expiring,
// so a heartbeat is only overdue past that point.
const backupHeartbeatMargin = 5 * time.Second

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
		if expiring := time.Duration(expiration)*time.Second - backupHeartbeatMargin; expiration > 0 && n.LastHeartbeat > 0 && now.Sub(time.Unix(n.LastHeartbeat, 0)) > expiring {
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
