// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package health builds a 'health' command to be used in binaries.
package health

import (
	"bytes"
	"encoding/json"
	"errors"
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
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	timeout    int
	JSON       bool
	PrettyJSON bool
	Issues     bool
	Detect     bool
}

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfFilePath         string
	ExtraConfFilePaths   []string
	ConfigName           string
	LoggerName           string
	FleetPoliciesDirPath string
}

// MakeCommand returns a `health` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}
	cmd := &cobra.Command{
		Use:          "health",
		Short:        "Print the current agent health",
		Long:         ``,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalParamsGetter()
			return fxutil.OneShot(requestHealth,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithConfigName(globalParams.ConfigName), config.WithExtraConfFiles(globalParams.ExtraConfFilePaths), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle(),
				secretsnoopfx.Module(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	cmd.Flags().IntVarP(&cliParams.timeout, "timeout", "t", 20, "timeout in second to query the Agent")
	cmd.Flags().BoolVarP(&cliParams.JSON, "json", "j", false, "print out raw json")
	cmd.Flags().BoolVarP(&cliParams.PrettyJSON, "pretty-json", "p", false, "pretty print JSON")
	cmd.Flags().BoolVarP(&cliParams.Issues, "issues", "i", false, "print detected health issues")
	cmd.Flags().BoolVarP(&cliParams.Detect, "detect", "d", false, "run health checks and print results")
	return cmd
}

// buildURL constructs the API URL for the given endpoint
func buildURL(ipcAddress string, config config.Component, endpoint string) string {
	if flavor.GetFlavor() == flavor.ClusterAgent {
		return fmt.Sprintf("https://%v:%v%s", ipcAddress, config.GetInt("cluster_agent.cmd_port"), endpoint)
	}
	return fmt.Sprintf("https://%v:%v%s", ipcAddress, config.GetInt("cmd_port"), endpoint)
}

// makeHTTPRequest makes an HTTP request and handles common error cases
func makeHTTPRequest(urlstr string, timeout time.Duration, client ipc.HTTPClient) ([]byte, error) {
	r, err := client.Get(urlstr, ipchttp.WithTimeout(timeout), ipchttp.WithCloseConnection)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		if e, found := errMap["error"]; found {
			err = errors.New(e)
		}
		return nil, fmt.Errorf("could not reach agent: %v \nMake sure the agent is running and health platform is enabled", err)
	}
	return r, nil
}

// handleJSONOutput handles JSON and pretty JSON output
func handleJSONOutput(data []byte, cliParams *cliParams) {
	if cliParams.PrettyJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, data, "", "  ") //nolint:errcheck
		data = prettyJSON.Bytes()
	}
	fmt.Print(string(data))
}

// printIssue prints a single issue in formatted output
func printIssue(checkID string, issueMap map[string]interface{}) {
	title := issueMap["Title"]
	severity := issueMap["Severity"]
	category := issueMap["Category"]

	severityColor := color.YellowString
	if sev, ok := severity.(string); ok && sev == "critical" {
		severityColor = color.RedString
	}

	fmt.Fprintf(color.Output, "\n%s (%s)\n", color.CyanString(checkID), severityColor(fmt.Sprint(severity)))
	fmt.Fprintf(color.Output, "  Title: %s\n", title)
	fmt.Fprintf(color.Output, "  Category: %s\n", category)

	if desc, ok := issueMap["Description"].(string); ok && desc != "" {
		fmt.Fprintf(color.Output, "  Description: %s\n", desc)
	}

	if tags, ok := issueMap["Tags"].([]interface{}); ok && len(tags) > 0 {
		tagStrs := make([]string, len(tags))
		for i, tag := range tags {
			tagStrs[i] = fmt.Sprint(tag)
		}
		fmt.Fprintf(color.Output, "  Tags: %s\n", strings.Join(tagStrs, ", "))
	}
}

func requestHealth(_ log.Component, config config.Component, cliParams *cliParams, client ipc.HTTPClient) error {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return err
	}

	// Handle --detect flag
	if cliParams.Detect {
		return requestHealthDetect(ipcAddress, config, cliParams, client)
	}

	// Handle --issues flag
	if cliParams.Issues {
		return requestHealthIssues(ipcAddress, config, cliParams, client)
	}

	// Build URL for standard health check
	urlstr := buildURL(ipcAddress, config, "/agent/status/health")
	if flavor.GetFlavor() == flavor.ClusterAgent {
		urlstr = buildURL(ipcAddress, config, "/status/health")
	}

	timeout := time.Duration(cliParams.timeout) * time.Second
	r, err := makeHTTPRequest(urlstr, timeout, client)
	if err != nil {
		return fmt.Errorf("could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues", err)
	}

	s := new(health.Status)
	if err = json.Unmarshal(r, s); err != nil {
		return fmt.Errorf("error unmarshalling json: %s", err)
	}

	// Handle JSON output
	if cliParams.JSON || cliParams.PrettyJSON {
		handleJSONOutput(r, cliParams)
		return nil
	}

	// Handle formatted text output
	sort.Strings(s.Unhealthy)
	sort.Strings(s.Healthy)

	statusString := color.GreenString("PASS")
	if len(s.Unhealthy) > 0 {
		statusString = color.RedString("FAIL")
	}
	fmt.Fprintf(color.Output, "Agent health: %s\n", statusString)

	if len(s.Healthy) > 0 {
		fmt.Fprintf(color.Output, "=== %s healthy components ===\n", color.GreenString(strconv.Itoa(len(s.Healthy))))
		fmt.Fprintln(color.Output, strings.Join(s.Healthy, ", "))
	}
	if len(s.Unhealthy) > 0 {
		fmt.Fprintf(color.Output, "=== %s unhealthy components ===\n", color.RedString(strconv.Itoa(len(s.Unhealthy))))
		fmt.Fprintln(color.Output, strings.Join(s.Unhealthy, ", "))
		return fmt.Errorf("found %d unhealthy components", len(s.Unhealthy))
	}

	return nil
}

func requestHealthIssues(ipcAddress string, config config.Component, cliParams *cliParams, client ipc.HTTPClient) error {
	urlstr := buildURL(ipcAddress, config, "/agent/health-issues")
	timeout := time.Duration(cliParams.timeout) * time.Second

	r, err := makeHTTPRequest(urlstr, timeout, client)
	if err != nil {
		return err
	}

	// Parse the response
	type IssueResponse struct {
		Count  int                    `json:"count"`
		Issues map[string]interface{} `json:"issues"`
	}

	var issueResp IssueResponse
	if err = json.Unmarshal(r, &issueResp); err != nil {
		return fmt.Errorf("error unmarshalling json: %s", err)
	}

	// Handle JSON output
	if cliParams.JSON || cliParams.PrettyJSON {
		handleJSONOutput(r, cliParams)
		return nil
	}

	// Handle formatted text output
	if issueResp.Count == 0 {
		fmt.Fprintf(color.Output, "Agent health issues: %s\n", color.GreenString("No issues detected"))
		return nil
	}

	fmt.Fprintf(color.Output, "Agent health issues: %s\n", color.YellowString(fmt.Sprintf("%d issue(s) detected", issueResp.Count)))
	fmt.Fprintf(color.Output, "\n=== Detected Issues ===\n")

	for checkID, issueData := range issueResp.Issues {
		if issueData == nil {
			continue
		}

		issueMap, ok := issueData.(map[string]interface{})
		if !ok {
			continue
		}

		printIssue(checkID, issueMap)
	}

	return nil
}

func requestHealthDetect(ipcAddress string, config config.Component, cliParams *cliParams, client ipc.HTTPClient) error {
	urlstr := buildURL(ipcAddress, config, "/agent/health-detect")
	timeout := time.Duration(cliParams.timeout) * time.Second

	r, err := makeHTTPRequest(urlstr, timeout, client)
	if err != nil {
		return err
	}

	// Parse the response
	type CheckResult struct {
		CheckID   string                 `json:"check_id"`
		CheckName string                 `json:"check_name"`
		IssueID   string                 `json:"issue_id,omitempty"`
		Status    string                 `json:"status"`
		Issue     map[string]interface{} `json:"issue,omitempty"`
	}

	type DetectResponse struct {
		Results []CheckResult `json:"results"`
	}

	var detectResp DetectResponse
	if err = json.Unmarshal(r, &detectResp); err != nil {
		return fmt.Errorf("error unmarshalling json: %s", err)
	}

	// Handle JSON output
	if cliParams.JSON || cliParams.PrettyJSON {
		handleJSONOutput(r, cliParams)
		return nil
	}

	// Handle formatted text output
	fmt.Fprintf(color.Output, "=== Health Check Detection Results ===\n")

	issuesFound := 0
	for _, result := range detectResp.Results {
		if result.IssueID != "" && result.Issue != nil {
			printIssue(result.CheckID, result.Issue)
			issuesFound++
		} else {
			fmt.Fprintf(color.Output, "\n%s: %s\n", color.CyanString(result.CheckName), color.GreenString("OK"))
			fmt.Fprintf(color.Output, "  Check ID: %s\n", result.CheckID)
			fmt.Fprintln(color.Output)
		}
	}

	// Print summary
	if issuesFound > 0 {
		fmt.Fprintf(color.Output, "\n%s detected across %d health check(s)\n",
			color.RedString(fmt.Sprintf("%d issue(s)", issuesFound)),
			len(detectResp.Results))
	} else {
		fmt.Fprintf(color.Output, "%s\n", color.GreenString("All health checks passed"))
	}

	return nil
}
