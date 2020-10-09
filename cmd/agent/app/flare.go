// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	pb "github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	remote_flare "github.com/DataDog/datadog-agent/pkg/flare/remote"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	customerEmail string
	autoconfirm   bool
	forceLocal    bool
	profiling     int

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
	AgentCmd.AddCommand(flareCmd)

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVarP(&forceLocal, "local", "l", false, "Force the creation of the flare by the command line instead of the agent process (useful when running in a containerized env)")
	flareCmd.Flags().IntVarP(&profiling, "profile", "p", -1, "Add performance profiling data to the flare. It will collect a heap profile and a CPU profile for the amount of seconds passed to the flag, with a minimum of 30s")
	flareCmd.SetArgs([]string{"caseID"})

	flareCmd.AddCommand(traceFlareCmd)
	traceFlareCmd.Flags().StringVarP(&tracerId, "tracer", "t", "", "Tracer identifier (use the tracer commands if unsure). (optional)")
	traceFlareCmd.Flags().StringVarP(&service, "service", "s", "", "Service you wish to collect trace logs for. (optional)")
	traceFlareCmd.Flags().StringVarP(&env, "env", "e", "", "Environment you wish to collect trace logs for. (optional)")
	traceFlareCmd.Flags().IntVarP(&tracing, "duration", "d", traceDurationDefault, "Duration in seconds of tracing logs collection. (optional)")
	traceFlareCmd.SetArgs([]string{"caseID"})

	traceFlareCmd.AddCommand(traceFlareQueryCmd)
	traceFlareQueryCmd.Flags().StringVarP(&service, "service", "s", "", "Tracers registered for this Service. (optional)")
	traceFlareQueryCmd.Flags().StringVarP(&env, "env", "e", "", "Tracers registered for this environment. (optional)")
}

var flareCmd = &cobra.Command{
	Use:   "flare [caseID]",
	Short: "Collect a flare and send it to Datadog",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		err = config.SetupLogger(loggerName, "off", "", "", false, true, false)
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

var traceFlareCmd = &cobra.Command{
	Use:   "trace [caseID]",
	Short: "Collect a trace flare and send it to Datadog",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		err = config.SetupLogger(loggerName, "off", "", "", false, true, false)
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

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		err = config.SetupLogger(loggerName, "off", "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		makeTraceQuery(service, env)

		return nil
	},
}

func readProfileData(pdata *flare.ProfileData) error {
	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from core.", profiling))
	coreDebugURL := fmt.Sprintf("http://127.0.0.1:%s/debug/pprof", config.Datadog.GetString("expvar_port"))
	if err := flare.CreatePerformanceProfile("core", coreDebugURL, profiling, pdata); err != nil {
		return err
	}

	if k := "apm_config.enabled"; config.Datadog.IsSet(k) && !config.Datadog.GetBool(k) {
		return nil
	}
	traceDebugURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", config.Datadog.GetInt("apm_config.receiver_port"))
	cpusec := 4 // 5s is the default maximum connection timeout on the trace-agent HTTP server
	if v := config.Datadog.GetInt("apm_config.receiver_timeout"); v > 0 {
		if v > profiling {
			// do not exceed requested duration
			cpusec = profiling
		} else {
			// fit within set limit
			cpusec = v - 1
		}
	}
	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from trace.", cpusec))
	return flare.CreatePerformanceProfile("trace", traceDebugURL, cpusec, pdata)
}

func makeFlare(caseID string) error {
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}
	var (
		profile flare.ProfileData
		err     error
	)
	if profiling >= 30 {
		if err := readProfileData(&profile); err != nil {
			fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Could not collect performance profile: %s", err)))
			return err
		}
	} else if profiling != -1 {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Invalid value for profiling: %d. Please enter an integer of at least 30.", profiling)))
		return err
	}

	var filePath string
	if forceLocal {
		filePath, err = createArchive(logFile, profile)
	} else {
		filePath, err = requestArchive(logFile, profile)
	}

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

func requestArchive(logFile string, pdata flare.ProfileData) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(logFile, pdata)
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/flare/core/gen", ipcAddress, config.Datadog.GetInt("cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", e)))
		return createArchive(logFile, pdata)
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
		return createArchive(logFile, pdata)
	}
	return string(r), nil
}

func requestTraceArchive(logFile string) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))

	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(logFile, nil)
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/flare/trace/gen", ipcAddress, config.Datadog.GetInt("cmd_port"))
	urlStatusStr := fmt.Sprintf("https://%v:%v/agent/flare/trace/status", ipcAddress, config.Datadog.GetInt("cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", e)))
		return createArchive(logFile, nil)
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
		return createArchive(logFile, nil)
	}

	for {
		e = json.Unmarshal(r, &status)
		if e != nil || status.Status == remote_flare.StatusUnknown {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(err.Error())))
			return createArchive(logFile, nil)
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
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	filePath, err := requestTraceArchive(logFile)
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

func createArchive(logFile string, pdata flare.ProfileData) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, e := flare.CreateArchive(true, common.GetDistPath(), common.PyChecksPath, logFile, pdata)
	if e != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", e)
		return "", e
	}
	return filePath, nil
}

func makeTraceQuery(service, env string) error {

	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return err
	}

	server := fmt.Sprintf("%v:%v", ipcAddress, config.Datadog.GetInt("cmd_port"))

	var tlsConf tls.Config
	tlsConf.InsecureSkipVerify = true
	opts := []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tlsConf))}

	conn, err := grpc.Dial(server, opts...)
	if err != nil {
		fmt.Printf("Failed trying to Dial: %v", err)
		return err
	}
	defer conn.Close()

	c := pb.NewAgentClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	r, err := c.FlareServiceQuery(ctx, &pb.FlareQueryRequest{
		Query: &pb.FlareHeartbeatRequest{
			TracerService:     service,
			TracerEnvironment: env,
		},
	})

	if err != nil {
		fmt.Printf("Failed after querying: %v", err)
		return err
	}

	for _, answer := range r.GetAnswer() {
		fmt.Printf("%v\t%v\t%v\n", answer.TracerIdentifier, answer.TracerService, answer.TracerEnvironment)
	}

	return nil

}
