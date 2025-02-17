// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Main package for the agent loader
package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"

	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/configcheck"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// The agent loader starts the trace-agent process when required,
// in particular only when data is received on the receivers.
// If anything goes wrong, the trace-agent is started directly.

func main() {
	listeners, err := getListeners()
	if err != nil {
		log.Warnf("Failed to pre-load the trace-agent: %v", err)
		err = unix.Exec(os.Args[1], os.Args[1:], os.Environ())
		if err != nil {
			log.Errorf("Failed to start the trace-agent: %v", err)
			os.Exit(1)
		}
	}

	if listeners == nil {
		log.Infof("Trace-agent is disabled, stopping...")
		return
	}

	if len(listeners) == 0 {
		log.Info("All trace-agent inputs are disabled, stopping...")
		return
	}

	var pollfds []unix.PollFd
	f := func() {
		log.Debugf("Polling... %+v", pollfds)
		n, err := unix.Poll(pollfds, -1)
		if err != nil {
			log.Warnf("error while polling: %v", err)
		} else {
			log.Debugf("Data received on %d sockets", n)
		}
	}

	for varname, ln := range listeners {
		f = func(varname string, ln net.Listener, f func()) func() {
			return func() {
				c, ok := ln.(syscall.Conn)
				if !ok {
					log.Warnf("Listener %s is not a syscall.Conn", varname)
					ln.Close()
					return
				}

				rc, err := c.SyscallConn()
				if err != nil {
					log.Warnf("syscallConn %s: %v", varname, err)
					ln.Close()
					return
				}

				err = rc.Control(func(fd uintptr) {
					log.Debugf("%s file descriptor is %d", varname, fd)
					os.Setenv(varname, fmt.Sprintf("%d", fd))

					flag, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
					if err != nil {
						log.Warnf("fcntl GETFD: %v\n", err)
						return
					}

					if flag&unix.FD_CLOEXEC != 0 {
						log.Debugf("Removing CLOEXEC on fd %v...\n", fd)
						_, err := unix.FcntlInt(fd, unix.F_SETFD, flag & ^unix.FD_CLOEXEC)
						if err != nil {
							log.Warnf("fcntl SETFD: %v\n", err)
							return
						}
					}

					pollfds = append(pollfds, unix.PollFd{
						Fd:     int32(fd),
						Events: unix.POLLIN,
					})
					f()
				})
				if err != nil {
					log.Warnf("control: %v", err)
				}
			}
		}(varname, ln, f)
	}

	f()

	// start the trace-agent whether there was an error or some data on a socket
	log.Info("starting the trace-agent...")
	err = unix.Exec(os.Args[1], os.Args[1:], os.Environ())
	if err != nil {
		log.Errorf("Failed to start the trace-agent: %v", err)
		os.Exit(1)
	}
}

// returns whether to start the trace-agent
func getListeners() (map[string]net.Listener, error) {
	cfg := pkgconfigsetup.Datadog()

	logparams := logdef.ForOneShot("TRACE-LOADER", cfg.GetString("log_level"), false)
	err := pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName(logparams.LoggerName()),
		logparams.LogLevelFn(cfg),
		logparams.LogFileFn(cfg),
		logparams.LogSyslogURIFn(cfg),
		logparams.LogSyslogRFCFn(cfg),
		logparams.LogToConsoleFn(cfg),
		logparams.LogFormatJSONFn(cfg),
		cfg,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the logger: %v", err)
	}

	// log.Debug("Loading trace-agent configuration")
	// tracecfg, err := traceconfig.LoadConfigFile(cfg.ConfigFileUsed(), cfg, nil)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to initialize configuration: %v", err)
	// }

	// from applyDatadogConfig in comp/trace/config/setup.go

	traceCfgEnabled := true
	if cfg.IsSet("apm_config.enabled") {
		traceCfgEnabled = utils.IsAPMEnabled(cfg)
	}

	traceCfgReceiverHost := "localhost"
	if cfg.IsSet("bind_host") || cfg.IsSet("apm_config.apm_non_local_traffic") {
		if cfg.IsSet("bind_host") {
			traceCfgReceiverHost = cfg.GetString("bind_host")
		}

		if cfg.IsSet("apm_config.apm_non_local_traffic") && cfg.GetBool("apm_config.apm_non_local_traffic") {
			traceCfgReceiverHost = "0.0.0.0"
		}
	} else if env.IsContainerized() {
		// Automatically activate non local traffic in containerized environment if no explicit config set
		log.Info("Activating non-local traffic automatically in containerized environment, trace-agent will listen on 0.0.0.0")
		traceCfgReceiverHost = "0.0.0.0"
	}

	traceCfgReceiverPort := 8126
	if cfg.IsSet("apm_config.receiver_port") {
		traceCfgReceiverPort = cfg.GetInt("apm_config.receiver_port")
	}

	traceCfgReceiverSocket := ""
	if runtime.GOOS == "linux" {
		traceCfgReceiverSocket = "/var/run/datadog/apm.socket"
	}
	if cfg.IsSet("apm_config.receiver_socket") {
		traceCfgReceiverSocket = cfg.GetString("apm_config.receiver_socket")
	}

	// end of config initialization

	if !traceCfgEnabled {
		return nil, nil
	}

	listeners := make(map[string]net.Listener)

	if traceCfgReceiverPort > 0 {
		log.Debugf("Listening to TCP receiver at port %d...", traceCfgReceiverPort)
		addr := net.JoinHostPort(traceCfgReceiverHost, strconv.Itoa(traceCfgReceiverPort))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return listeners, fmt.Errorf("error listening to tcp receiver: %v", err)
		}

		listeners["DD_APM_NET_RECEIVER_FD"] = ln
	} else {
		log.Info("Tracer-agent TCP receiver is disabled")
	}

	if path := traceCfgReceiverSocket; path != "" {
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			log.Debugf("Listening to unix receiver at path %s", path)
			ln, err := net.Listen("unix", path)
			if err != nil {
				return listeners, fmt.Errorf("error listening to unix receiver: %v", err)
			}

			listeners["DD_APM_UNIX_RECEIVER_FD"] = ln
		} else {
			log.Errorf("Could not start UDS listener: socket directory does not exist: %s", path)
		}
	} else {
		log.Info("Trace unix receiver is disabled")
	}

	if configcheck.IsEnabled(cfg) {
		grpcPort := cfg.GetInt(pkgconfigsetup.OTLPTracePort)
		log.Debugf("Listening to otlp port %d", grpcPort)
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", traceCfgReceiverHost, grpcPort))
		if err != nil {
			return listeners, fmt.Errorf("error listening to otlp receiver: %v", err)
		}

		listeners["DD_OTLP_CONFIG_GRPC_FD"] = ln
	} else {
		log.Info("OTLP trace receiver is disabled")
	}

	return listeners, nil
}
