// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// GetClusterChecks dumps the clustercheck dispatching state to the writer
func GetClusterChecks(w io.Writer, checkName string) error {
	urlstr := fmt.Sprintf("https://localhost:%v/api/v1/clusterchecks", config.Datadog.GetInt("cluster_agent.cmd_port"))

	if w != color.Output {
		color.NoColor = true
	}

	if !config.Datadog.GetBool("cluster_checks.enabled") {
		fmt.Fprintln(w, "Cluster-checks are not enabled")
		return nil
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	r, err := util.DoGet(c, urlstr, util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(w, fmt.Sprintf("The agent ran into an error while checking config: %s", string(r)))
		} else {
			fmt.Fprintln(w, fmt.Sprintf("Failed to query the agent (running?): %s", err))
		}
		return err
	}

	var cr types.StateResponse
	err = json.Unmarshal(r, &cr)
	if err != nil {
		return err
	}

	// Gracefully exit when dispatcher is not running
	if len(cr.NotRunning) > 0 {
		fmt.Fprintf(w, "Cluster-check dispatching logic not running: %s\n", cr.NotRunning)
		return nil
	}

	// Print warmup message
	if cr.Warmup {
		fmt.Fprintln(w, fmt.Sprintf("=== %s in progress ===", color.BlueString("Warmup")))
		fmt.Fprintln(w, "No configuration has been processed yet")
		fmt.Fprintln(w, "")
	}

	// Print dangling configs
	if len(cr.Dangling) > 0 {
		fmt.Fprintln(w, fmt.Sprintf("=== %s configurations ===", color.RedString("Unassigned")))
		for _, c := range cr.Dangling {
			PrintConfig(w, c, checkName)
		}
		fmt.Fprintln(w, "")
	}

	// Print summary of agents
	if len(cr.Nodes) == 0 {
		fmt.Fprintln(w, fmt.Sprintf("=== %s agent reporting ===", color.RedString("Zero")))
		fmt.Fprintln(w, "No check will be dispatched until agents report to the cluster-agent")
		return nil
	}
	fmt.Fprintln(w, fmt.Sprintf("=== %d agents reporting ===", len(cr.Nodes)))
	sort.Slice(cr.Nodes, func(i, j int) bool { return cr.Nodes[i].Name < cr.Nodes[j].Name })
	table := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(table, "\nName\tRunning checks")
	for _, n := range cr.Nodes {
		fmt.Fprintf(table, "%s\t%d\n", n.Name, len(n.Configs))
	}
	table.Flush()

	// Print per-node configurations
	for _, node := range cr.Nodes {
		if len(node.Configs) == 0 {
			continue
		}
		fmt.Fprintln(w, fmt.Sprintf("\n===== Checks on %s =====", color.HiMagentaString(node.Name)))
		for _, c := range node.Configs {
			PrintConfig(w, c, checkName)
		}
	}

	return nil
}

// GetEndpointsChecks dumps the endpointschecks dispatching state to the writer
func GetEndpointsChecks(w io.Writer, checkName string) error {
	if !endpointschecksEnabled() {
		return nil
	}

	urlstr := fmt.Sprintf("https://localhost:%v/api/v1/endpointschecks/configs", config.Datadog.GetInt("cluster_agent.cmd_port"))

	if w != color.Output {
		color.NoColor = true
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	if err := util.SetAuthToken(); err != nil {
		return err
	}

	// Query the cluster agent API
	r, err := util.DoGet(c, urlstr, util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(w, fmt.Sprintf("The agent ran into an error while checking config: %s", string(r)))
		} else {
			fmt.Fprintln(w, fmt.Sprintf("Failed to query the agent (running?): %s", err))
		}
		return err
	}

	var cr types.ConfigResponse
	if err = json.Unmarshal(r, &cr); err != nil {
		return err
	}

	// Print summary of pod-backed endpointschecks
	fmt.Fprintln(w, fmt.Sprintf("\n===== %d Pod-backed Endpoints-Checks scheduled =====", len(cr.Configs)))
	for _, c := range cr.Configs {
		PrintConfig(w, c, checkName)
	}

	return nil
}

func endpointschecksEnabled() bool {
	for _, provider := range config.Datadog.GetStringSlice("extra_config_providers") {
		if provider == names.KubeEndpointsRegisterName {
			return true
		}
	}
	return false
}
