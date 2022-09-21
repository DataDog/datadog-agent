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

	"github.com/fatih/color"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/input"
)

var (
	customerEmail        string
	autoconfirm          bool
	forceLocal           bool
	profiling            int
	profileMutex         bool
	profileMutexFraction int
	profileBlocking      bool
	profileBlockingRate  int
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	flareCmd := &cobra.Command{
		Use:   "flare [caseID]",
		Short: "Collect a flare and send it to Datadog",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {

			err := common.SetupConfig(globalParams.ConfFilePath)
			if err != nil {
				return fmt.Errorf("unable to set up global agent configuration: %v", err)
			}

			// The flare command should not log anything, all errors should be reported directly to the console without the log format
			err = config.SetupLogger(config.CoreLoggerName, "off", "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}

			caseID := ""
			if len(args) > 0 {
				caseID = args[0]
			}

			if customerEmail == "" {
				var err error
				customerEmail, err = input.AskForEmail()
				if err != nil {
					fmt.Println("Error reading email, please retry or contact support")
					return err
				}
			}

			return makeFlare(caseID)
		},
	}

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVarP(&forceLocal, "local", "l", false, "Force the creation of the flare by the command line instead of the agent process (useful when running in a containerized env)")
	flareCmd.Flags().IntVarP(&profiling, "profile", "p", -1, "Add performance profiling data to the flare. It will collect a heap profile and a CPU profile for the amount of seconds passed to the flag, with a minimum of 30s")
	flareCmd.Flags().BoolVarP(&profileMutex, "profile-mutex", "M", false, "Add mutex profile to the performance data in the flare")
	flareCmd.Flags().IntVarP(&profileMutexFraction, "profile-mutex-fraction", "", 100, "Set the fraction of mutex contention events that are reported in the mutex profile")
	flareCmd.Flags().BoolVarP(&profileBlocking, "profile-blocking", "B", false, "Add gorouting blocking profile to the performance data in the flare")
	flareCmd.Flags().IntVarP(&profileBlockingRate, "profile-blocking-rate", "", 10000, "Set the fraction of goroutine blocking events that are reported in the blocking profile")
	flareCmd.SetArgs([]string{"caseID"})

	return []*cobra.Command{flareCmd}
}

type profileCollector func(prefix, debugURL string, cpusec int, target *flare.ProfileData) error
type agentProfileCollector func(pdata *flare.ProfileData, seconds int, c profileCollector) error

func readProfileData(pdata *flare.ProfileData, seconds int, collector profileCollector) error {
	prevSettings, err := setRuntimeProfilingSettings()
	if err != nil {
		return err
	}
	defer resetRuntimeProfilingSettings(prevSettings)

	agentCollectors := []struct {
		name string
		fn   agentProfileCollector
	}{
		{
			name: "core",
			fn:   readCoreAgentProfileData,
		},
		{
			name: "trace",
			fn:   readTraceAgentProfileData,
		},
		{
			name: "process",
			fn:   readProcessAgentProfileData,
		},
	}

	var errs error
	for _, c := range agentCollectors {
		if err := c.fn(pdata, seconds, collector); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("error collecting %s agent profile: %v", c.name, err))
		}
	}

	return errs
}

func readCoreAgentProfileData(pdata *flare.ProfileData, seconds int, collector profileCollector) error {
	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from core.", profiling))
	coreDebugURL := fmt.Sprintf("http://127.0.0.1:%s/debug/pprof", config.Datadog.GetString("expvar_port"))
	return collector("core", coreDebugURL, seconds, pdata)
}

func readTraceAgentProfileData(pdata *flare.ProfileData, seconds int, collector profileCollector) error {
	if k := "apm_config.enabled"; config.Datadog.IsSet(k) && !config.Datadog.GetBool(k) {
		return nil
	}
	traceDebugURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", config.Datadog.GetInt("apm_config.receiver_port"))
	cpusec := 4 // 5s is the default maximum connection timeout on the trace-agent HTTP server
	if v := config.Datadog.GetInt("apm_config.receiver_timeout"); v > 0 {
		if v > seconds {
			// do not exceed requested duration
			cpusec = seconds
		} else {
			// fit within set limit
			cpusec = v - 1
		}
	}
	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from trace.", cpusec))
	return collector("trace", traceDebugURL, cpusec, pdata)
}

func readProcessAgentProfileData(pdata *flare.ProfileData, seconds int, collector profileCollector) error {
	// We are unconditionally collecting process agent profile in the flare as best effort
	processDebugURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", config.Datadog.GetInt("process_config.expvar_port"))
	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from process.", profiling))
	return collector("process", processDebugURL, seconds, pdata)
}

func makeFlare(caseID string) error {
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}
	jmxLogFile := config.Datadog.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = common.DefaultJmxLogFile
	}
	logFiles := []string{logFile, jmxLogFile}
	var (
		profile flare.ProfileData
		err     error
	)
	if profiling >= 30 {
		if err := readProfileData(&profile, profiling, flare.CreatePerformanceProfile); err != nil {
			fmt.Fprintln(color.Output, color.YellowString(fmt.Sprintf("Could not collect performance profile data: %s", err)))
		}
	} else if profiling != -1 {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Invalid value for profiling: %d. Please enter an integer of at least 30.", profiling)))
		return err
	}

	var filePath string
	if forceLocal {
		filePath, err = createArchive(logFiles, profile, nil)
	} else {
		filePath, err = requestArchive(logFiles, profile)
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
	if !autoconfirm {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintln(color.Output, fmt.Sprintf("Aborting. (You can still use %s)", color.YellowString(filePath)))
			return nil
		}
	}

	response, e := flare.SendFlare(filePath, caseID, customerEmail)
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

func requestArchive(logFiles []string, pdata flare.ProfileData) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(logFiles, pdata, err)
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/flare", ipcAddress, config.Datadog.GetInt("cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", e)))
		return createArchive(logFiles, pdata, e)
	}

	p, err := json.Marshal(pdata)
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error while encoding profile: %s", e)))
		return "", err
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer(p))
	if e != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(string(r))))
			e = fmt.Errorf("Error getting flare from running agent: %s", r)
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
			e = fmt.Errorf("Error getting flare from running agent: %w", e)
		}
		return createArchive(logFiles, pdata, e)
	}
	return string(r), nil
}

func createArchive(logFiles []string, pdata flare.ProfileData, ipcError error) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, e := flare.CreateArchive(true, common.GetDistPath(), common.PyChecksPath, logFiles, pdata, ipcError)
	if e != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", e)
		return "", e
	}
	return filePath, nil
}

func setRuntimeProfilingSettings() (map[string]interface{}, error) {
	prev := make(map[string]interface{})
	if profileMutex && profileMutexFraction > 0 {
		old, err := setRuntimeSetting("runtime_mutex_profile_fraction", profileMutexFraction)
		if err != nil {
			return nil, err
		}
		prev["runtime_mutex_profile_fraction"] = old
	}
	if profileBlocking && profileBlockingRate > 0 {
		old, err := setRuntimeSetting("runtime_block_profile_rate", profileBlockingRate)
		if err != nil {
			return nil, err
		}
		prev["runtime_block_profile_rate"] = old
	}
	return prev, nil
}

func setRuntimeSetting(name string, new int) (interface{}, error) {
	c, err := common.NewSettingsClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize settings client: %v", err)
	}

	fmt.Fprintln(color.Output, color.BlueString("Setting %s to %v", name, new))

	if err := util.SetAuthToken(); err != nil {
		return nil, fmt.Errorf("unable to set up authentication token: %v", err)
	}

	oldVal, err := c.Get(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get current value of %s: %v", name, err)
	}

	if _, err := c.Set(name, fmt.Sprint(new)); err != nil {
		return nil, fmt.Errorf("failed to set %s to %v: %v", name, new, err)
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
