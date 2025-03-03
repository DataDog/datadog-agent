// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"

	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/configcheck"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// The agent loader starts the trace-agent process when required,
// in particular only when data is received on the receivers.
// If anything goes wrong, the trace-agent is started directly.

// os.Args[1] is the path to the configuration file
// os.Args[2] is the path to the trace-agent binary
// os.Args[3:] are the arguments to the trace-agent command

func main() {
	cfg := pkgconfigsetup.Datadog()
	cfg.SetConfigFile(os.Args[1])
	_, err := pkgconfigsetup.LoadDatadogCustom(cfg, "datadog.yaml", option.None[secrets.Component](), nil)
	if err != nil {
		log.Warnf("Failed to load the configuration: %v", err)
		execOrExit(os.Environ())
	}

	logparams := logdef.ForOneShot("TRACE-LOADER", cfg.GetString("log_level"), false)
	err = pkglogsetup.SetupLogger(
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
		log.Warnf("Failed to initialize the logger: %v", err)
		execOrExit(os.Environ())
	}

	// log.Debug("Loading trace-agent configuration")
	// tracecfg, err := traceconfig.LoadConfigFile(cfg.ConfigFileUsed(), cfg, nil)
	// if err != nil {
	// 	log.Warnf("Failed to initialize trace-agent configuration: %v", err)
	// 	execOrExit(os.Environ())
	// }

	listeners, err := getListeners(cfg)
	if err != nil {
		log.Warnf("Failed to pre-load the trace-agent: %v", err)
		execOrExit(os.Environ())
	}

	if listeners == nil {
		log.Infof("Trace-agent is disabled, stopping...")
		return
	}

	if len(listeners) == 0 {
		log.Info("All trace-agent inputs are disabled, stopping...")
		return
	}

	env := os.Environ()
	var pollfds []unix.PollFd
	for varname, fd := range listeners {
		log.Debugf("%s file descriptor is %d", varname, fd)
		env = append(env, fmt.Sprintf("%s=%d", varname, fd))
		pollfds = append(pollfds, unix.PollFd{
			Fd:     int32(fd),
			Events: unix.POLLIN,
		})
	}

	log.Debugf("Polling... %+v", pollfds)
	n, err := unix.Poll(pollfds, -1)
	if err != nil {
		log.Warnf("error while polling: %v", err)
	} else {
		log.Debugf("Data received on %d sockets", n)
	}

	// start the trace-agent whether there was an error or some data on a socket
	execOrExit(env)
}

func execOrExit(env []string) {
	log.Info("Starting the trace-agent...")
	err := unix.Exec(os.Args[2], os.Args[2:], env)
	log.Errorf("Failed to start the trace-agent: %v", err)
	os.Exit(1)
}

// returns whether to start the trace-agent
func getListeners(cfg model.Reader) (map[string]uintptr, error) {
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

	listeners := make(map[string]uintptr)

	if traceCfgReceiverPort > 0 {
		log.Debugf("Listening to TCP receiver at port %d...", traceCfgReceiverPort)
		addr := net.JoinHostPort(traceCfgReceiverHost, strconv.Itoa(traceCfgReceiverPort))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return listeners, fmt.Errorf("error listening to tcp receiver: %v", err)
		}
		defer ln.Close()

		fd, err := fdFromListener(ln)
		if err != nil {
			return listeners, fmt.Errorf("error getting file descriptor from tcp listener: %v", err)
		}

		listeners["DD_APM_NET_RECEIVER_FD"] = fd
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
			defer ln.Close()

			fd, err := fdFromListener(ln)
			if err != nil {
				return listeners, fmt.Errorf("error getting file descriptor from unix listener: %v", err)
			}

			listeners["DD_APM_UNIX_RECEIVER_FD"] = fd
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
		defer ln.Close()

		fd, err := fdFromListener(ln)
		if err != nil {
			return listeners, fmt.Errorf("error getting file descriptor from otlp listener: %v", err)
		}

		listeners["DD_OTLP_CONFIG_GRPC_FD"] = fd
	} else {
		log.Info("OTLP trace receiver is disabled")
	}

	return listeners, nil
}

func fdFromListener(ln net.Listener) (uintptr, error) {
	lnf, ok := ln.(interface {
		File() (*os.File, error)
	})
	if !ok {
		return 0, errors.New("listener does not support File()")
	}

	f, err := lnf.File()
	if err != nil {
		return 0, fmt.Errorf("failed to get file from listener: %v", err)
	}

	fd := f.Fd()

	flag, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
	if err != nil {
		return 0, fmt.Errorf("fcntl GETFD: %v", err)
	}

	if flag&unix.FD_CLOEXEC != 0 {
		log.Debugf("Removing CLOEXEC on fd %v...\n", fd)
		_, err := unix.FcntlInt(fd, unix.F_SETFD, flag & ^unix.FD_CLOEXEC)
		if err != nil {
			return 0, fmt.Errorf("fcntl SETFD: %v", err)
		}
	}

	return fd, nil
}
