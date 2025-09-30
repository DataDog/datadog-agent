// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status implements 'agent status'.
package status

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args are the positional command-line arguments
	args []string

	jsonStatus         bool
	prettyPrintJSON    bool
	statusFilePath     string
	verbose            bool
	list               bool
	logLevelDefaultOff command.LogLevelDefaultOff
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	cmd := &cobra.Command{
		Use:   "status [section]",
		Short: "Display the current status",
		Long: `Display the current status.
If no section is specified, this command will display all status sections.
If a specific section is provided, such as 'collector', it will only display the status of that section.
The --list flag can be used to list all available status sections.`,
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.args = args

			// Prevent autoconfig to run when running status as it logs before logger
			// is setup.  Cannot rely on config.Override as env detection is run before
			// overrides are set.  TODO: This should eventually be handled with a
			// BundleParams field for AD.
			os.Setenv("DD_AUTOCONFIG_FROM_ENVIRONMENT", "false")

			return fxutil.OneShot(statusCmd,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:            log.ForOneShot(command.LoggerName, cliParams.logLevelDefaultOff.Value(), true)}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	cliParams.logLevelDefaultOff.Register(cmd)
	cmd.PersistentFlags().BoolVarP(&cliParams.jsonStatus, "json", "j", false, "print out raw json")
	cmd.PersistentFlags().BoolVarP(&cliParams.prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	cmd.PersistentFlags().StringVarP(&cliParams.statusFilePath, "file", "o", "", "Output the status command to a file")
	cmd.PersistentFlags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "print out verbose status")
	cmd.PersistentFlags().BoolVarP(&cliParams.list, "list", "l", false, "list all available status sections")

	return []*cobra.Command{cmd}
}

func scrubMessage(message string) string {
	msgScrubbed, err := scrubber.ScrubBytes([]byte(message))
	if err == nil {
		return string(msgScrubbed)
	}
	return "[REDACTED] - failure to clean the message"
}

func redactError(unscrubbedError error) error {
	if unscrubbedError == nil {
		return unscrubbedError
	}

	errMsg := unscrubbedError.Error()
	scrubbedMsg, scrubOperationErr := scrubber.ScrubBytes([]byte(errMsg))
	var scrubbedError error
	if scrubOperationErr != nil {
		scrubbedError = errors.New("[REDACTED] failed to clean error")
	} else {
		scrubbedError = errors.New(string(scrubbedMsg))
	}

	return scrubbedError
}

func statusCmd(logger log.Component, _ sysprobeconfig.Component, cliParams *cliParams, client ipc.HTTPClient) error {
	if cliParams.list {
		return redactError(requestSections(client))
	}

	if len(cliParams.args) < 1 {
		return redactError(requestStatus(cliParams, client))
	}

	return componentStatusCmd(logger, cliParams, client)
}

func setIpcURL(cliParams *cliParams) url.Values {
	v := url.Values{}
	if cliParams.verbose {
		v.Set("verbose", "true")
	}

	if cliParams.prettyPrintJSON || cliParams.jsonStatus {
		v.Set("format", "json")
	} else {
		v.Set("format", "text")
	}

	return v
}

func renderResponse(res []byte, cliParams *cliParams) error {
	var s string

	// The rendering is done in the client so that the agent has less work to do
	if cliParams.prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, res, "", "  ") //nolint:errcheck
		s = prettyJSON.String()
	} else if cliParams.jsonStatus {
		s = string(res)
	} else {
		s = scrubMessage(string(res))
	}

	if cliParams.statusFilePath != "" {
		return os.WriteFile(cliParams.statusFilePath, []byte(s), 0644)
	}
	fmt.Println(s)
	return nil
}

func requestStatus(cliParams *cliParams, client ipc.HTTPClient) error {

	if !cliParams.prettyPrintJSON && !cliParams.jsonStatus {
		fmt.Printf("Getting the status from the agent.\n\n")
	}

	v := setIpcURL(cliParams)

	endpoint, err := client.NewIPCEndpoint("/agent/status")
	if err != nil {
		return err
	}

	res, err := endpoint.DoGet(ipchttp.WithValues(v))
	if err != nil {
		return err
	}

	// The rendering is done in the client so that the agent has less work to do
	err = renderResponse(res, cliParams)
	if err != nil {
		return err
	}

	return nil
}

func componentStatusCmd(_ log.Component, cliParams *cliParams, client ipc.HTTPClient) error {
	if len(cliParams.args) > 1 {
		return fmt.Errorf("only one section must be specified")
	}

	return redactError(componentStatus(cliParams, cliParams.args[0], client))
}

func componentStatus(cliParams *cliParams, component string, client ipc.HTTPClient) error {

	v := setIpcURL(cliParams)

	endpoint, err := client.NewIPCEndpoint(fmt.Sprintf("/agent/%s/status", component))
	if err != nil {
		return err
	}
	res, err := endpoint.DoGet(ipchttp.WithValues(v))
	if err != nil {
		return err
	}

	// The rendering is done in the client so that the agent has less work to do
	err = renderResponse(res, cliParams)
	if err != nil {
		return err
	}

	return nil
}

func requestSections(client ipc.HTTPClient) error {
	endpoint, err := client.NewIPCEndpoint("/agent/status/sections")
	if err != nil {
		return err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return err
	}

	var sections []string
	err = json.Unmarshal(res, &sections)
	if err != nil {
		return err
	}

	for _, section := range sections {
		fmt.Printf("- \"%s\"\n", section)
	}

	return nil
}
