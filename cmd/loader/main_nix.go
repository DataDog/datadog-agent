// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"

	delegatedauthnooptypes "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl/types"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/configcheck"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/trace/api/loader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// The agent loader starts the trace-agent process when required,
// in particular only when connections are established on its sockets.
// If anything goes wrong, the trace-agent is started directly.

// os.Args[1] is the path to the configuration file
// os.Args[2] is the path to the trace-agent binary
// os.Args[3:] are the arguments to the trace-agent command

func main() {
	// check if the trace-agent path is absolute or found in the PATH
	fullPath, err := exec.LookPath(os.Args[2])
	if err != nil {
		log.Criticalf("Failed to look up the trace-agent binary: %v", err)
		os.Exit(1)
	}

	cfg := pkgconfigsetup.GlobalConfigBuilder()
	cfg.SetConfigFile(os.Args[1])
	err = pkgconfigsetup.LoadDatadog(cfg, &secretnooptypes.SecretNoop{}, &delegatedauthnooptypes.DelegatedAuthNoop{}, nil)
	if err != nil {
		log.Warnf("Failed to load the configuration: %v", err)
		execOrExit(os.Environ(), fullPath)
	}

	// comp/trace/config/config*.go
	logFile := "/var/log/datadog/trace-agent.log"
	if runtime.GOOS == "darwin" {
		logFile = "/opt/datadog-agent/logs/trace-agent.log"
	}
	// cmd/trace-agent/subcommands/run/command.go
	logparams := logdef.ForDaemon("TRACE-LOADER", "apm_config.log_file", logFile)
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
		execOrExit(os.Environ(), fullPath)
	}

	if !utils.IsAPMEnabled(cfg) {
		log.Infof("Trace-agent is disabled, stopping...")
		return
	}

	if !cfg.GetBool("apm_config.socket_activation.enabled") {
		log.Infof("Socket-activation for the trace-agent is disabled, running the trace-agent directly...")
		execOrExit(os.Environ(), fullPath)
	}

	tcpFD, listeners, err := getListeners(cfg)
	if err != nil {
		log.Warnf("Failed to get listeners for the trace-agent: %v", err)
		for name, fd := range listeners {
			log.Debugf("Closing file descriptor %d for %s", fd, name)
			err := unix.Close(int(fd))
			if err != nil {
				log.Warnf("Failed to close file descriptor %s: %v", name, err)
			}
		}
		execOrExit(os.Environ(), fullPath)
	}

	if len(listeners) == 0 {
		log.Info("All trace-agent inputs are disabled, stopping...")
		return
	}

	if !cfg.GetBool("apm_config.socket_activation.handle_tcp_probe") {
		// if we don't try to handle the TCP probe, we can just tcpFD to an invalid value
		// it will never be equal to one of the file descriptors and we'll never try to accept on it
		// we still poll on the TCP file descriptor (in listeners)
		tcpFD = -1
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

	// most of the work is done, we'll just poll (ie. wait) and exec, so we flush the memory
	// so that the binary appears to use as little memory as possible
	releaseMemory()

	// we don't want to wake up the trace-agent due to the liveness probe on the TCP listener,
	// so we accept connections on that socket and wait for them to close,
	// if they don't then we exec the trace-agent directly

	log.Infof("Polling... %+v", pollfds)
	tcpClientFD := -1
	for {
		n, err := unix.Poll(pollfds, -1)
		if err != nil {
			log.Warnf("error while polling: %v", err)
			break
		}

		log.Debugf("Events received on %d sockets", n)
		var tcpFdHasEvents bool
		var tcpClientFdHasEvents bool
		for _, pfd := range pollfds {
			if pfd.Revents != 0 {
				log.Debugf("Socket %d has events %s", pfd.Fd, reventToString(pfd.Revents))
				if pfd.Fd == int32(tcpFD) {
					tcpFdHasEvents = true
				}
				if pfd.Fd == int32(tcpClientFD) {
					tcpClientFdHasEvents = true
				}
			}
		}

		// exec the trace-agent if:
		// - there are multiple events
		// - there is a tcp client connected and the event is not associated with it
		// - there is no tcp client connected and the event is not associated with the TCP listener

		if n > 1 {
			log.Infof("Received %d events, executing trace-agent...", n)
			break
		}

		if tcpClientFD != -1 && !tcpClientFdHasEvents {
			log.Info("There is a tcp client connected and the event is not associated with it, executing trace-agent...")
			break
		}

		// this condition is always true if we don't try to handle the TCP probe:
		// - tcpFD is -1
		// - so tcpFdHasEvents is always false
		// - and tcpClientFD is also -1
		if tcpClientFD == -1 && !tcpFdHasEvents {
			log.Info("There is no tcp client connected and the event is not associated with the TCP listener, executing trace-agent...")
			break
		}

		if tcpClientFdHasEvents {
			// peek at the data to see if the client has directly closed the connection
			var buf [1]byte
			n, _, err := unix.Recvfrom(tcpClientFD, buf[:], unix.MSG_PEEK)
			if err != nil {
				log.Warnf("error while peeking at TCP client: %v", err)
				break
			}

			// the client has sent data, execute the trace-agent
			if n > 0 {
				log.Info("TCP client sent data, executing trace-agent...")
				break
			}

			// the client has closed the connection, close the file descriptor and start over
			log.Debugf("TCP client closed connection, closing file descriptor %d", tcpClientFD)
			err = unix.Close(tcpClientFD)
			if err != nil {
				log.Warnf("error while closing TCP client file descriptor: %v", err)
				break
			}

			// remove the environment variable and pollfd for the TCP client file descriptor
			env = env[:len(env)-1]
			pollfds = pollfds[:len(pollfds)-1]
			tcpClientFD = -1
			continue
		}

		// the event is associated with the TCP listener, accept the connection
		tcpClientFD, _, err = unix.Accept(tcpFD)
		if err != nil {
			log.Warnf("error while accepting on TCP listener: %v", err)
			break
		}

		// get a file descriptor that can be passed to child processes using the exec syscall
		// and set the environment variable to let the trace-agent know about it
		err = loader.MakeExecutable(uintptr(tcpClientFD))
		if err != nil {
			log.Warnf("error making file descriptor executable: %v", err)
			_ = unix.Close(tcpClientFD)
			break
		}

		log.Debugf("Accepted connection on TCP listener: %d", tcpClientFD)
		env = append(env, fmt.Sprintf("DD_APM_NET_RECEIVER_CLIENT_FD=%d", tcpClientFD))
		pollfds = append(pollfds, unix.PollFd{
			Fd:     int32(tcpClientFD),
			Events: unix.POLLIN,
		})
	}

	// start the trace-agent whether there was an error or some data on a socket
	execOrExit(env, fullPath)
}

// Returns a string representation of the events that occurred on a socket
// Only POLLIN and error events are managed
func reventToString(revents int16) string {
	var ret string
	if unix.POLLIN&revents != 0 {
		ret += "POLLIN "
	}
	if unix.POLLERR&revents != 0 {
		ret += "POLLERR "
	}
	if unix.POLLHUP&revents != 0 {
		ret += "POLLHUP "
	}
	if unix.POLLNVAL&revents != 0 {
		// would be a programming error (file descriptor is not valid / open)
		ret += "POLLNVAL "
	}

	return ret
}

func execOrExit(env []string, fullPath string) {
	log.Info("Starting the trace-agent...")
	log.Tracef("Starting the trace-agent with env: %+q", env)
	log.Flush()
	err := unix.Exec(fullPath, os.Args[2:], env)
	log.Errorf("Failed to start the trace-agent with args %+q: %v", os.Args[2:], err)
	log.Flush()
	os.Exit(1)
}

// returns a map of environment variables to file descriptors that the trace-agent will use
// the first return value is the file descriptor of the TCP listener or -1 if not enabled,
// so that it can be handled specially to manage the liveness probe
func getListeners(cfg model.Reader) (tcpFD int, listeners map[string]uintptr, err error) {
	// logic from applyDatadogConfig in comp/trace/config/setup.go
	// the loader needs to initialize the sockets in the same way as the trace-agent

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

	tcpFD = -1
	listeners = make(map[string]uintptr)

	// "datadog" TCP receiver
	if traceCfgReceiverPort > 0 {
		log.Infof("Listening to TCP receiver at port %d...", traceCfgReceiverPort)
		addr := net.JoinHostPort(traceCfgReceiverHost, strconv.Itoa(traceCfgReceiverPort))
		ln, err := loader.GetTCPListener(addr)
		if err != nil {
			return 0, listeners, fmt.Errorf("error listening to tcp receiver: %v", err)
		}
		defer ln.Close()

		fd, err := loader.GetFDFromListener(ln)
		if err != nil {
			return 0, listeners, fmt.Errorf("error getting file descriptor from tcp listener: %v", err)
		}

		tcpFD = int(fd)
		listeners["DD_APM_NET_RECEIVER_FD"] = fd
	} else {
		log.Info("Trace-agent TCP receiver is disabled")
	}

	// "datadog" UDS receiver
	if path := traceCfgReceiverSocket; path != "" {
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			log.Infof("Listening to unix receiver at path %s", path)

			ln, err := loader.GetUnixListener(path)
			if err != nil {
				return tcpFD, listeners, fmt.Errorf("error listening to unix receiver: %v", err)
			}
			defer ln.Close()

			fd, err := loader.GetFDFromListener(ln)
			if err != nil {
				return tcpFD, listeners, fmt.Errorf("error getting file descriptor from unix listener: %v", err)
			}

			listeners["DD_APM_UNIX_RECEIVER_FD"] = fd
		} else {
			log.Errorf("Could not start UDS listener: socket directory does not exist: %s", path)
		}
	} else {
		log.Info("Trace-agent unix receiver is disabled")
	}

	// OTLP TCP receiver
	if configcheck.IsConfigEnabled(cfg) {
		grpcPort := cfg.GetInt(pkgconfigsetup.OTLPTracePort)
		log.Infof("Listening to otlp port %d", grpcPort)
		ln, err := loader.GetTCPListener(fmt.Sprintf("%s:%d", traceCfgReceiverHost, grpcPort))
		if err != nil {
			return tcpFD, listeners, fmt.Errorf("error listening to otlp receiver: %v", err)
		}
		defer ln.Close()

		fd, err := loader.GetFDFromListener(ln)
		if err != nil {
			return tcpFD, listeners, fmt.Errorf("error getting file descriptor from otlp listener: %v", err)
		}

		listeners["DD_OTLP_CONFIG_GRPC_FD"] = fd
	} else {
		log.Info("Trace-agent OTLP receiver is disabled")
	}

	return tcpFD, listeners, nil
}
