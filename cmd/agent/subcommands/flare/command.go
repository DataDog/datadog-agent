// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements 'agent flare'.
package flare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/fatih/color"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args are the positional command-line arguments
	args []string

	// subcommand-specific flags

	customerEmail        string
	autoconfirm          bool
	forceLocal           bool
	profiling            int
	profileMutex         bool
	profileMutexFraction int
	profileBlocking      bool
	profileBlockingRate  int
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	flareCmd := &cobra.Command{
		Use:   "flare [caseID]",
		Short: "Collect a flare and send it to Datadog",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			config := config.NewAgentParamsWithSecrets(globalParams.ConfFilePath,
				config.WithSecurityAgentConfigFilePaths([]string{
					path.Join(commonpath.DefaultConfPath, "security-agent.yaml"),
				}),
				config.WithConfigLoadSecurityAgent(true),
				config.WithIgnoreErrors(true))

			return fxutil.OneShot(makeFlare,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config,
					SysprobeConfigParams: sysprobeconfig.NewParams(sysprobeconfig.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
					LogParams:            log.LogForOneShot(command.LoggerName, "off", false),
				}),
				fx.Supply(flare.NewLocalParams(
					commonpath.GetDistPath(),
					commonpath.PyChecksPath,
					commonpath.DefaultLogFile,
					commonpath.DefaultJmxLogFile,
				)),
				flare.Module,
				core.Bundle,
			)
		},
	}

	flareCmd.Flags().StringVarP(&cliParams.customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&cliParams.autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVarP(&cliParams.forceLocal, "local", "l", false, "Force the creation of the flare by the command line instead of the agent process (useful when running in a containerized env)")
	flareCmd.Flags().IntVarP(&cliParams.profiling, "profile", "p", -1, "Add performance profiling data to the flare. It will collect a heap profile and a CPU profile for the amount of seconds passed to the flag, with a minimum of 30s")
	flareCmd.Flags().BoolVarP(&cliParams.profileMutex, "profile-mutex", "M", false, "Add mutex profile to the performance data in the flare")
	flareCmd.Flags().IntVarP(&cliParams.profileMutexFraction, "profile-mutex-fraction", "", 100, "Set the fraction of mutex contention events that are reported in the mutex profile")
	flareCmd.Flags().BoolVarP(&cliParams.profileBlocking, "profile-blocking", "B", false, "Add gorouting blocking profile to the performance data in the flare")
	flareCmd.Flags().IntVarP(&cliParams.profileBlockingRate, "profile-blocking-rate", "", 10000, "Set the fraction of goroutine blocking events that are reported in the blocking profile")
	flareCmd.SetArgs([]string{"caseID"})

	return []*cobra.Command{flareCmd}
}

func readProfileData(seconds int) (flare.ProfileData, error) {
	type agentProfileCollector func(service string) error

	pdata := flare.ProfileData{}
	c := apiutil.GetClient(false)

	serviceProfileCollector := func(portConfig string, seconds int) agentProfileCollector {
		return func(service string) error {
			fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from %s.", seconds, service))
			pprofURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", pkgconfig.Datadog.GetInt(portConfig))

			for _, prof := range []struct{ name, URL string }{
				{
					// 1st heap profile
					name: service + "-1st-heap.pprof",
					URL:  pprofURL + "/heap",
				},
				{
					// CPU profile
					name: service + "-cpu.pprof",
					URL:  fmt.Sprintf("%s/profile?seconds=%d", pprofURL, seconds),
				},
				{
					// 2nd heap profile
					name: service + "-2nd-heap.pprof",
					URL:  pprofURL + "/heap",
				},
				{
					// mutex profile
					name: service + "-mutex.pprof",
					URL:  pprofURL + "/mutex",
				},
				{
					// goroutine blocking profile
					name: service + "-block.pprof",
					URL:  pprofURL + "/block",
				},
			} {
				b, err := apiutil.DoGet(c, prof.URL, apiutil.LeaveConnectionOpen)
				if err != nil {
					return err
				}
				pdata[prof.name] = b
			}
			return nil
		}
	}

	agentCollectors := map[string]agentProfileCollector{
		"core":           serviceProfileCollector("expvar_port", seconds),
		"security-agent": serviceProfileCollector("security_agent.expvar_port", seconds),
	}

	if pkgconfig.Datadog.GetBool("process_config.enabled") ||
		pkgconfig.Datadog.GetBool("process_config.container_collection.enabled") ||
		pkgconfig.Datadog.GetBool("process_config.process_collection.enabled") {

		agentCollectors["process"] = serviceProfileCollector("process_config.expvar_port", seconds)
	}

	if pkgconfig.Datadog.GetBool("apm_config.enabled") {
		traceCpusec := pkgconfig.Datadog.GetInt("apm_config.receiver_timeout")
		if traceCpusec > seconds {
			// do not exceed requested duration
			traceCpusec = seconds
		} else if traceCpusec <= 0 {
			// default to 4s as maximum connection timeout of trace-agent HTTP server is 5s by default
			traceCpusec = 4
		}

		agentCollectors["trace"] = serviceProfileCollector("apm_config.debug.port", traceCpusec)
	}

	if debugPort := pkgconfig.Datadog.GetInt("system_probe_config.debug_port"); debugPort > 0 {
		agentCollectors["system-probe"] = serviceProfileCollector("system_probe_config.debug_port", seconds)
	}

	var errs error
	for name, callback := range agentCollectors {
		if err := callback(name); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("error collecting %s agent profile: %v", name, err))
		}
	}

	return pdata, errs
}

func makeFlare(flareComp flare.Component, log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component, cliParams *cliParams) error {
	var (
		profile flare.ProfileData
		err     error
	)

	warnings := config.Warnings()
	if warnings != nil && warnings.Err != nil {
		fmt.Fprintln(color.Error, color.YellowString("Config parsing warning: %v", warnings.Err))
	}
	caseID := ""
	if len(cliParams.args) > 0 {
		caseID = cliParams.args[0]
	}

	customerEmail := cliParams.customerEmail
	if customerEmail == "" {
		customerEmail, err = input.AskForEmail()
		if err != nil {
			fmt.Println("Error reading email, please retry or contact support")
			return err
		}
	}

	if cliParams.profiling >= 30 {
		resetPreviousSettings, err := setRuntimeProfilingSettings(cliParams)
		if err != nil {
			return err
		}

		if profile, err = readProfileData(cliParams.profiling); err != nil {
			fmt.Fprintln(color.Output, color.YellowString(fmt.Sprintf("Could not collect performance profile data: %s", err)))
		}

		resetPreviousSettings()
	} else if cliParams.profiling != -1 {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Invalid value for profiling: %d. Please enter an integer of at least 30.", cliParams.profiling)))
		return err
	}

	var filePath string
	if cliParams.forceLocal {
		filePath, err = createArchive(flareComp, profile, nil)
	} else {
		filePath, err = requestArchive(flareComp, profile)
	}

	if err != nil {
		return err
	}

	if _, err := os.Stat(filePath); err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("The flare zipfile \"%s\" does not exist.", filePath)))
		fmt.Fprintln(color.Output, color.RedString("If the agent running in a different container try the '--local' option to generate the flare locally"))
		return err
	}

	fmt.Fprintln(color.Output, fmt.Sprintf("%s is going to be uploaded to Datadog", color.YellowString(filePath)))
	if !cliParams.autoconfirm {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintln(color.Output, fmt.Sprintf("Aborting. (You can still use %s)", color.YellowString(filePath)))
			return nil
		}
	}

	response, e := flareComp.Send(filePath, caseID, customerEmail)
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

func requestArchive(flareComp flare.Component, pdata flare.ProfileData) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(flareComp, pdata, err)
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/flare", ipcAddress, pkgconfig.Datadog.GetInt("cmd_port"))

	// Set session token
	if err = util.SetAuthToken(); err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", err)))
		return createArchive(flareComp, pdata, err)
	}

	p, err := json.Marshal(pdata)
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error while encoding profile: %s", err)))
		return "", err
	}

	r, err := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer(p))
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(string(r))))
			err = fmt.Errorf("Error getting flare from running agent: %s", r)
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
			err = fmt.Errorf("Error getting flare from running agent: %w", err)
		}
		return createArchive(flareComp, pdata, err)
	}

	return string(r), nil
}

func createArchive(flareComp flare.Component, pdata flare.ProfileData, ipcError error) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, err := flareComp.Create(pdata, ipcError)
	if err != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", err)
		return "", err
	}

	return filePath, nil
}

func setRuntimeProfilingSettings(cliParams *cliParams) (func(), error) {
	c, err := common.NewSettingsClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize settings client: %v", err)
	}
	if err := util.SetAuthToken(); err != nil {
		return nil, fmt.Errorf("unable to set up authentication token: %v", err)
	}

	prev := make(map[string]interface{})
	if cliParams.profileMutex && cliParams.profileMutexFraction > 0 {
		old, err := setRuntimeSetting(c, "runtime_mutex_profile_fraction", cliParams.profileMutexFraction)
		if err != nil {
			return nil, err
		}
		prev["runtime_mutex_profile_fraction"] = old
	}
	if cliParams.profileBlocking && cliParams.profileBlockingRate > 0 {
		old, err := setRuntimeSetting(c, "runtime_block_profile_rate", cliParams.profileBlockingRate)
		if err != nil {
			return nil, err
		}
		prev["runtime_block_profile_rate"] = old
	}

	return func() { resetRuntimeProfilingSettings(prev) }, nil
}

func setRuntimeSetting(c settings.Client, name string, value int) (interface{}, error) {
	fmt.Fprintln(color.Output, color.BlueString("Setting %s to %v", name, value))

	oldVal, err := c.Get(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get current value of %s: %v", name, err)
	}

	if _, err := c.Set(name, fmt.Sprint(value)); err != nil {
		return nil, fmt.Errorf("failed to set %s to %v: %v", name, value, err)
	}

	return oldVal, nil
}

func resetRuntimeProfilingSettings(prev map[string]interface{}) {
	if len(prev) == 0 {
		return
	}

	c, err := common.NewSettingsClient()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString("Failed to restore runtime settings: %v", err))
		return
	}

	for name, value := range prev {
		fmt.Fprintln(color.Output, color.BlueString("Restoring %s to %v", name, value))
		if _, err := c.Set(name, fmt.Sprint(value)); err != nil {
			fmt.Fprintln(color.Output, color.RedString("Failed to restore previous value of %s: %v", name, err))
		}
	}
}
