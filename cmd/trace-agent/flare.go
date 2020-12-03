// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package main

import (
	"bytes"
	// "context"
	// "crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	// pb "github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	core_config "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	remote_flare "github.com/DataDog/datadog-agent/pkg/flare/remote"
	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/util/input"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	customerEmail string
	autoconfirm   bool

	tracerId string
	service  string
	env      string
	tracing  int
)

const (
	// TODO: this 50MB upper bound should perhaps be defined elsewhere
	maxFlareSize         = 50 * 1024 * 1024
	traceDurationDefault = 30
)

func init() {
	flags.TraceCmd.AddCommand(flareCmd)

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&autoconfirm, "send", "z", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().StringVarP(&tracerId, "tracer", "t", "", "Tracer identifier (use the tracer commands if unsure). (optional)")
	flareCmd.Flags().StringVarP(&service, "service", "s", "", "Service you wish to collect trace logs for. (optional)")
	flareCmd.Flags().StringVarP(&env, "env", "v", "", "Environment you wish to collect trace logs for. (optional)")
	flareCmd.Flags().IntVarP(&tracing, "duration", "d", traceDurationDefault, "Duration in seconds of tracing logs collection. (optional)")
	flareCmd.SetArgs([]string{"caseID"})

	flareCmd.AddCommand(traceFlareQueryCmd)
	traceFlareQueryCmd.Flags().StringVarP(&service, "service", "s", "", "Tracers registered for this Service. (optional)")
	traceFlareQueryCmd.Flags().StringVarP(&env, "env", "e", "", "Tracers registered for this environment. (optional)")
}

var flareCmd = &cobra.Command{
	Use:   "flare [caseID]",
	Short: "Collect a trace flare and send it to Datadog",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		// will this fly in the trace-agent container?
		err := common.SetupConfig(flags.ConfigPath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		err = core_config.SetupLogger("TRACE", "off", "", "", false, true, false)
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

		return makeTraceFlare(caseID)
	},
}

var traceFlareQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Collect a flare and send it to Datadog",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		err := common.SetupConfig(flags.ConfigPath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		err = core_config.SetupLogger("TRACE", "off", "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		makeTraceQuery(service, env)

		return nil
	},
}

func makeFlare(caseID string) error {
	logFile := core_config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}
	jmxLogFile := core_config.Datadog.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = common.DefaultJmxLogFile
	}
	logFiles := []string{logFile, jmxLogFile}

	filePath, err := requestArchive(logFiles, nil)

	if err != nil {
		return err
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("The flare zipfile \"%s\" does not exist.", filePath)))
		fmt.Fprintln(color.Output, color.RedString("If the agent running in a different container try the '--local' option to generate the flare locally"))
		return err
	}

	// check for max size
	if stat.Size() > maxFlareSize {
		warn := fmt.Sprintf("The flare \"%s\" is too large (%d bytes) to upload, please submit manually to our support team", filePath, stat.Size())
		fmt.Fprintln(color.Output, color.RedString(warn))
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
	ipcAddress, err := core_config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(logFiles, pdata)
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/flare/core/gen", ipcAddress, core_config.Datadog.GetInt("cmd_port"))

	// Set session token
	e = security.SetAuthToken()
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", e)))
		return createArchive(logFiles, pdata)
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
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
		}
		return createArchive(logFiles, pdata)
	}
	return string(r), nil
}

func requestTraceArchive(logFiles []string) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))

	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := core_config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(logFiles, nil)
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/flare/trace/gen", ipcAddress, core_config.Datadog.GetInt("cmd_port"))
	urlStatusStr := fmt.Sprintf("https://%v:%v/agent/flare/trace/status", ipcAddress, core_config.Datadog.GetInt("cmd_port"))

	// Set session token
	e = security.SetAuthToken()
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", e)))
		return createArchive(logFiles, nil)
	}

	form := url.Values{}

	form.Add("tracer_id", tracerId)
	form.Add("environment", env)
	form.Add("service", service)
	form.Add("duration", fmt.Sprintf("%d", tracing))

	var status remote_flare.RemoteFlareStatus
	r, e := util.DoPost(c, urlstr, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if e != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(string(r))))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
		}
		return createArchive(logFiles, nil)
	}

	for {
		e = json.Unmarshal(r, &status)
		if e != nil || status.Status == remote_flare.StatusUnknown {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(err.Error())))
			return createArchive(logFiles, nil)
		}

		if status.Status != remote_flare.StatusReady {
			if status.Ttl > 0 {
				time.Sleep(time.Duration(status.Ttl) * time.Second)
			}

			form := url.Values{}
			form.Add("flare_id", status.Id)
			r, e = util.DoPost(c, urlStatusStr, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		} else {
			break
		}
	}

	return status.File, nil
}

func makeTraceFlare(caseID string) error {
	logFile := core_config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	filePath, err := requestTraceArchive([]string{logFile})
	if err != nil {
		return err
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("The flare zipfile \"%s\" does not exist.", filePath)))
		fmt.Fprintln(color.Output, color.RedString("If the agent running in a different container try the '--local' option to generate the flare locally"))
		return err
	}

	// check for max size
	if stat.Size() > maxFlareSize {
		warn := fmt.Sprintf("The flare \"%s\" is too large (%d bytes) to upload, please submit manually to our support team", filePath, stat.Size())
		fmt.Fprintln(color.Output, color.RedString(warn))
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

func createArchive(logFiles []string, pdata flare.ProfileData) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, e := flare.CreateArchive(true, common.GetDistPath(), common.PyChecksPath, logFiles, pdata)
	if e != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", e)
		return "", e
	}
	return filePath, nil
}

func makeTraceQuery(service, env string) error {

	// TODO: move to trace agent
	// ipcAddress, err := config.GetIPCAddress()
	// if err != nil {
	// 	fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
	// 	return err
	// }

	// server := fmt.Sprintf("%v:%v", ipcAddress, config.Datadog.GetInt("cmd_port"))

	// var tlsConf tls.Config
	// tlsConf.InsecureSkipVerify = true
	// opts := []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tlsConf))}

	// conn, err := grpc.Dial(server, opts...)
	// if err != nil {
	// 	fmt.Printf("Failed trying to Dial: %v", err)
	// 	return err
	// }
	// defer conn.Close()

	// c := pb.NewAgentClient(conn)

	// ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	// defer cancel()

	// r, err := c.FlareServiceQuery(ctx, &pb.FlareQueryRequest{
	// 	Query: &pb.FlareHeartbeatRequest{
	// 		TracerService:     service,
	// 		TracerEnvironment: env,
	// 	},
	// })

	// if err != nil {
	// 	fmt.Printf("Failed after querying: %v", err)
	// 	return err
	// }

	// for _, answer := range r.GetAnswer() {
	// 	fmt.Printf("%v\t%v\t%v\n", answer.TracerIdentifier, answer.TracerService, answer.TracerEnvironment)
	// }

	return nil

}
