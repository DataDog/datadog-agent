// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dcaflare

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/fatih/color"
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
	cmd.Flags().BoolVarP(&cliParams.profileBlocking, "profile-blocking", "B", false, "Add goroutine blocking profile to the performance data in the flare")
	cmd.Flags().IntVarP(&cliParams.profileBlockingRate, "profile-blocking-rate", "", 10000, "Set the fraction of goroutine blocking events that are reported in the blocking profile")
	cmd.SetArgs([]string{"caseID"})

	return cmd
}

func readProfileData(seconds int) (flare.ProfileData, error) {
	pdata := flare.ProfileData{}
	c := apiutil.GetClient(false)

	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from datadog-cluster-agent.", seconds))
	pprofURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", pkgconfig.Datadog.GetInt("expvar_port"))

	for _, prof := range []struct{ name, URL string }{
		{
			// 1st heap profile
			name: "datadog-cluster-agent-1st-heap.pprof",
			URL:  pprofURL + "/heap",
		},
		{
			// CPU profile
			name: "datadog-cluster-agent-cpu.pprof",
			URL:  fmt.Sprintf("%s/profile?seconds=%d", pprofURL, seconds),
		},
		{
			// 2nd heap profile
			name: "datadog-cluster-agent-2nd-heap.pprof",
			URL:  pprofURL + "/heap",
		},
		{
			// mutex profile
			name: "datadog-cluster-agent-mutex.pprof",
			URL:  pprofURL + "/mutex",
		},
		{
			// goroutine blocking profile
			name: "datadog-cluster-agent-block.pprof",
			URL:  pprofURL + "/block",
		},
	} {
		b, err := apiutil.DoGet(c, prof.URL, apiutil.LeaveConnectionOpen)
		if err != nil {
			return pdata, err
		}
		pdata[prof.name] = b
	}

	return pdata, nil
}

func run(log log.Component, config config.Component, cliParams *cliParams) error {
	fmt.Fprintln(color.Output, color.BlueString("Asking the Cluster Agent to build the flare archive."))
	var (
		profile flare.ProfileData
		e       error
	)
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/flare", pkgconfig.Datadog.GetInt("cluster_agent.cmd_port"))

	logFile := pkgconfig.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = path.DefaultDCALogFile
	}

	if cliParams.profiling >= 30 {
		settingsClient, err := newSettingsClient()
		if err != nil {
			return fmt.Errorf("failed to initialize settings client: %v", err)
		}

		profilingOpts := settings.ProfilingOpts{
			ProfileMutex:         cliParams.profileMutex,
			ProfileMutexFraction: cliParams.profileMutexFraction,
			ProfileBlocking:      cliParams.profileBlocking,
			ProfileBlockingRate:  cliParams.profileBlockingRate,
		}

		e = settings.ExecWithRuntimeProfilingSettings(func() {
			if profile, e = readProfileData(cliParams.profiling); e != nil {
				fmt.Fprintln(color.Output, color.YellowString(fmt.Sprintf("Could not collect performance profile data: %s", e)))
			}
		}, profilingOpts, settingsClient)
		if e != nil {
			return e
		}
	} else if cliParams.profiling != -1 {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Invalid value for profiling: %d. Please enter an integer of at least 30.", cliParams.profiling)))
		return nil
	}

	if e = util.SetAuthToken(); e != nil {
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
			fmt.Fprintf(color.Output, "The agent ran into an error while making the flare: %s\n", color.RedString(string(r)))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make a full flare: %s.", e.Error()))
		}
		fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally, some logs will be missing."))
		filePath, e = flare.CreateDCAArchive(true, path.GetDistPath(), logFile, aggregator.GetSenderManager(), profile)
		if e != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", e)
			return e
		}
	} else {
		filePath = string(r)
	}

	fmt.Fprintf(color.Output, "%s is going to be uploaded to Datadog\n", color.YellowString(filePath))
	if !cliParams.send {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintf(color.Output, "Aborting. (You can still use %s)\n", color.YellowString(filePath))
			return nil
		}
	}

	response, e := flare.SendFlare(filePath, cliParams.caseID, cliParams.email, helpers.NewLocalFlareSource())
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

func newSettingsClient() (settings.Client, error) {
	c := util.GetClient(false)

	apiConfigURL := fmt.Sprintf(
		"https://localhost:%v/config",
		pkgconfig.Datadog.GetInt("cluster_agent.cmd_port"),
	)

	return settingshttp.NewClient(c, apiConfigURL, "datadog-cluster-agent"), nil
}
