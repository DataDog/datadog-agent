// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

var elog debug.Log

// ServiceName is the service name used for the process-agent
const ServiceName = "datadog-process-agent"

// opts are the command-line options
var defaultConfigPath = flags.DefaultConfPath
var defaultSysProbeConfigPath = flags.DefaultSysProbeConfPath
var defaultConfdPath = flags.DefaultConfdPath
var defaultLogFilePath = flags.DefaultLogFilePath

var winopts struct {
	installService   bool
	uninstallService bool
	startService     bool
	stopService      bool
	foreground       bool
}

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfigPath = filepath.Join(pd, "datadog.yaml")
		defaultSysProbeConfigPath = filepath.Join(pd, "system-probe.yaml")
		defaultConfdPath = filepath.Join(pd, "conf.d")
		defaultLogFilePath = filepath.Join(pd, "logs", "process-agent.log")
	}
}

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	exit := make(chan struct{})

	go func() {
		for {
			select {
			case c := <-r:
				switch c.Cmd {
				case svc.Interrogate:
					changes <- c.CurrentStatus
					// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
					time.Sleep(100 * time.Millisecond)
					changes <- c.CurrentStatus
				case svc.Stop, svc.PreShutdown, svc.Shutdown:
					elog.Info(0x40000006, ServiceName)
					changes <- svc.Status{State: svc.StopPending}
					///// FIXME:  Need a way to indicate to rest of service to shut
					////  down
					close(exit)
					break
				default:
					elog.Warning(0xc000000A, fmt.Sprint(c.Cmd))
				}
			}
		}
	}()
	elog.Info(0x40000003, ServiceName)
	runAgent(exit)

	changes <- svc.Status{State: svc.Stopped}
	return
}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(ServiceName)
	} else {
		elog, err = eventlog.Open(ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	elog.Info(0x40000007, ServiceName)
	err = run(ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, ServiceName)
}

// main is the main application entry point
func main() {
	rootCmd.PersistentFlags().StringVar(&opts.configPath, flags.CfgPath, defaultConfigPath, "Path to datadog.yaml config")
	rootCmd.PersistentFlags().StringVar(&opts.sysProbeConfigPath, flags.SysProbeConfig, defaultSysProbeConfigPath, "Path to system-probe.yaml config")
	rootCmd.PersistentFlags().BoolVarP(&opts.info, "info", "i", false, "Show info about running process agent and exit")
	rootCmd.PersistentFlags().BoolP("version", "v", false, "[deprecated] Print the version and exit")
	rootCmd.PersistentFlags().String("check", "", "[deprecated] Run a specific check and print the results. Choose from: process, rtprocess, container, rtcontainer, connections, process_discovery")

	// windows-specific options for installing the service, uninstalling the service, etc.
	rootCmd.PersistentFlags().BoolVar(&winopts.installService, "install-service", false, "Install the process agent to the Service Control Manager")
	rootCmd.PersistentFlags().BoolVar(&winopts.uninstallService, "uninstall-service", false, "Remove the process agent from the Service Control Manager")
	rootCmd.PersistentFlags().BoolVar(&winopts.startService, "start-service", false, "Starts the process agent service")
	rootCmd.PersistentFlags().BoolVar(&winopts.stopService, "stop-service", false, "Stops the process agent service")
	rootCmd.PersistentFlags().BoolVar(&winopts.foreground, "foreground", false, "Always run foreground instead whether session is interactive or not")

	// Invoke the Agent
	os.Args = fixDeprecatedFlags(os.Args, os.Stdout)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}

func rootCmdRun(cmd *cobra.Command, args []string) {
	if !winopts.foreground {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}

		if !isIntSess {
			runService(false)
			return
		}
		// sigh.  Go doesn't have boolean xor operator.  The options are mutually exclusive,
		// make sure more than one wasn't specified
		optcount := 0
		if winopts.installService {
			fmt.Println("Installservice")
			optcount++
		} else {
			fmt.Println("no Installservice")
		}
		if winopts.uninstallService {
			optcount++
		}
		if winopts.startService {
			optcount++
		}
		if winopts.stopService {
			optcount++
		}
		if optcount > 1 {
			fmt.Println("Incompatible options chosen")
			return
		}
		if winopts.installService {
			if err = installService(); err != nil {
				fmt.Printf("Error installing service %v\n", err)
			}
			return
		}
		if winopts.uninstallService {
			if err = removeService(); err != nil {
				fmt.Printf("Error removing service %v\n", err)
			}
			return
		}
		if winopts.startService {
			if err = startService(); err != nil {
				fmt.Printf("Error starting service %v\n", err)
			}
			return
		}
		if winopts.stopService {
			if err = stopService(); err != nil {
				fmt.Printf("Error stopping service %v\n", err)
			}
			return
		}
	}

	exit := make(chan struct{})
	// Invoke the Agent
	runAgent(exit)
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func stopService() error {
	return controlService(svc.Stop, svc.Stopped)
}

func restartService() error {
	var err error
	if err = stopService(); err == nil {
		err = startService()
	}
	return err
}

func controlService(c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}

func installService() error {
	exepath, err := exePath()
	if err != nil {
		return err
	}
	fmt.Printf("exepath: %s\n", exepath)

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", ServiceName)
	}
	s, err = m.CreateService(ServiceName, exepath, mgr.Config{DisplayName: "Datadog Agent Service"})
	if err != nil {
		return err
	}
	defer s.Close()
	err = eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		s.Delete()
		return fmt.Errorf("SetupEventLogSource() failed: %s", err)
	}
	return nil
}

func exePath() (string, error) {
	prog := os.Args[0]
	p, err := filepath.Abs(prog)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(p)
	if err == nil {
		if !fi.Mode().IsDir() {
			return p, nil
		}
		err = fmt.Errorf("%s is directory", p)
	}
	if filepath.Ext(p) == "" {
		p += ".exe"
		fi, err := os.Stat(p)
		if err == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}
			return "", fmt.Errorf("%s is directory", p)
		}
	}
	return "", err
}

func removeService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", ServiceName)
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	err = eventlog.Remove(ServiceName)
	if err != nil {
		return fmt.Errorf("RemoveEventLogSource() failed: %s", err)
	}
	return nil
}
