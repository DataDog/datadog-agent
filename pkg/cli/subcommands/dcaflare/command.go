// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package dcaflare

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	pkgflare "github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/fatih/color"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type cliParams struct {
	caseID               string
	email                string
	send                 bool
	profiling            int
	profileMutex         bool
	profileMutexFraction int
	profileBlocking      bool
	profileBlockingRate  int
}

type GlobalParams struct {
	ConfFilePath string
}

const (
	LoggerName      = "CLUSTER"
	DefaultLogLevel = "off"
)

// MakeCommand returns a `flare` command to be used by cluster-agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "flare [caseID]",
		Short: "Collect a flare and send it to Datadog",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				cliParams.caseID = args[0]
			}

			if cliParams.email == "" {
				var err error
				cliParams.email, err = input.AskForEmail()
				if err != nil {
					fmt.Println("Error reading email, please retry or contact support")
					return err
				}
			}
			globalParams := globalParamsGetter()

			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath, config.WithConfigLoadSecrets(true)),
					LogParams:    log.LogForOneShot(LoggerName, DefaultLogLevel, true),
				}),
				core.Bundle,
			)
		},
	}

	cmd.Flags().StringVarP(&cliParams.email, "email", "e", "", "Your email")
	cmd.Flags().BoolVarP(&cliParams.send, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	cmd.Flags().IntVarP(&cliParams.profiling, "profile", "p", -1, "Add performance profiling data to the flare. It will collect a heap profile and a CPU profile for the amount of seconds passed to the flag, with a minimum of 30s")
	cmd.Flags().BoolVarP(&cliParams.profileMutex, "profile-mutex", "M", false, "Add mutex profile to the performance data in the flare")
	cmd.Flags().IntVarP(&cliParams.profileMutexFraction, "profile-mutex-fraction", "", 100, "Set the fraction of mutex contention events that are reported in the mutex profile")
	cmd.Flags().BoolVarP(&cliParams.profileBlocking, "profile-blocking", "B", false, "Add gorouting blocking profile to the performance data in the flare")
	cmd.Flags().IntVarP(&cliParams.profileBlockingRate, "profile-blocking-rate", "", 10000, "Set the fraction of goroutine blocking events that are reported in the blocking profile")
	cmd.SetArgs([]string{"caseID"})

	return cmd
}

type profileCollector func(prefix, debugURL string, cpusec int, target *pkgflare.ProfileDataDCA) error
type agentProfileCollector func(cliParams *cliParams, pdata *pkgflare.ProfileDataDCA, c profileCollector) error

func readProfileData(cliParams *cliParams, pdata *pkgflare.ProfileDataDCA, seconds int, collector profileCollector) error {
	prevSettings, err := setRuntimeProfilingSettings(cliParams)
	if err != nil {
		return err
	}
	defer resetRuntimeProfilingSettings(prevSettings)

	type agentCollector struct {
		name string
		fn   agentProfileCollector
	}

	agentCollectors := []agentCollector{{
		name: "dca",
		fn:   serviceProfileCollector("dca", "expvar_port", seconds),
	}}

	var errs error
	for _, c := range agentCollectors {
		if err := c.fn(cliParams, pdata, collector); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("error collecting %s agent profile: %v", c.name, err))
		}
	}

	return errs
}

func serviceProfileCollector(service string, portConfig string, seconds int) agentProfileCollector {
	return func(cliParams *cliParams, pdata *pkgflare.ProfileDataDCA, collector profileCollector) error {
		fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from %s.", seconds, service))
		pprofURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", pkgconfig.Datadog.GetInt(portConfig))
		return collector(service, pprofURL, seconds, pdata)
	}
}

func run(log log.Component, config config.Component, cliParams *cliParams) error {
	fmt.Fprintln(color.Output, color.BlueString("Asking the Cluster Agent to build the flare archive."))

	var (
		profile pkgflare.ProfileDataDCA
		e       error
	)

	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/flare", pkgconfig.Datadog.GetInt("cluster_agent.cmd_port"))

	logFile := pkgconfig.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = path.DefaultDCALogFile
	}

	if cliParams.profiling >= 30 {
		if err := readProfileData(cliParams, &profile, cliParams.profiling, pkgflare.CreatePerformanceProfileDCA); err != nil {
			fmt.Fprintln(color.Output, color.YellowString(fmt.Sprintf("Could not collect performance profile data: %s", err)))
		}
	} else if cliParams.profiling != -1 {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Invalid value for profiling: %d. Please enter an integer of at least 30.", cliParams.profiling)))
		return e
	}

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	p, e := json.Marshal(profile)
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error while encoding profile: %s", e)))
		return e
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer(p))

	var filePath string
	if e != nil {

		if r != nil && string(r) != "" {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(string(r))))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make a full flare: %s.", e.Error()))
		}
		fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally, some logs will be missing."))
		filePath, e = flare.CreateDCAArchive(true, path.GetDistPath(), logFile, profile)
		if e != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", e)
			return e
		}
	} else {
		filePath = string(r)
	}

	fmt.Fprintln(color.Output, fmt.Sprintf("%s is going to be uploaded to Datadog", color.YellowString(filePath)))
	if !cliParams.send {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintln(color.Output, fmt.Sprintf("Aborting. (You can still use %s)", color.YellowString(filePath)))
			return nil
		}
	}

	response, e := flare.SendFlare(filePath, cliParams.caseID, cliParams.email)
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

func setRuntimeProfilingSettings(cliParams *cliParams) (map[string]interface{}, error) {
	prev := make(map[string]interface{})
	if cliParams.profileMutex && cliParams.profileMutexFraction > 0 {
		old, err := setRuntimeSetting("runtime_mutex_profile_fraction", cliParams.profileMutexFraction)
		if err != nil {
			return nil, err
		}
		prev["runtime_mutex_profile_fraction"] = old
	}
	if cliParams.profileBlocking && cliParams.profileBlockingRate > 0 {
		old, err := setRuntimeSetting("runtime_block_profile_rate", cliParams.profileBlockingRate)
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
