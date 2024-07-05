// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

//nolint:revive // TODO(SERV) Fix revive linter
package mode

import (
	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/afero"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// Run is the entrypoint of the init process. It will spawn the customer process
func RunInit(logConfig *serverlessLog.Config) {
	if len(os.Args) < 2 {
		panic("[datadog init process] invalid argument count, did you forget to set CMD ?")
	}

	args := os.Args[1:]

	log.Debugf("Launching subprocess %v\n", args)
	err := execute(logConfig, args)
	if err != nil {
		log.Debugf("Error exiting: %v\n", err)
	}
}

func execute(logConfig *serverlessLog.Config, args []string) error {
	commandName, commandArgs := buildCommandParam(args)

	// Add our tracer settings
	fs := afero.NewOsFs()
	autoInstrumentTracer(fs)

	cmd := exec.Command(commandName, commandArgs...)

	cmd.Stdout = io.Writer(os.Stdout)
	cmd.Stderr = io.Writer(os.Stderr)

	if logConfig.IsEnabled {
		cmd.Stdout = io.MultiWriter(os.Stdout, serverlessLog.NewChannelWriter(logConfig.Channel, false))
		cmd.Stderr = io.MultiWriter(os.Stderr, serverlessLog.NewChannelWriter(logConfig.Channel, true))
	}

	err := cmd.Start()
	if err != nil {
		return err
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs)
	go forwardSignals(cmd.Process, sigs)
	err = cmd.Wait()
	return err
}

func buildCommandParam(cmdArg []string) (string, []string) {
	fields := cmdArg
	if len(cmdArg) == 1 {
		fields = strings.Fields(cmdArg[0])
	}
	commandName := fields[0]
	if len(fields) > 1 {
		return commandName, fields[1:]
	}
	return commandName, []string{}
}

func forwardSignals(process *os.Process, sigs chan os.Signal) {
	for sig := range sigs {
		if sig != syscall.SIGCHLD {
			if process != nil {
				_ = syscall.Kill(process.Pid, sig.(syscall.Signal))
			}
		}
	}
}

// Tracer holds a name, a path to the trace directory, and an
// initialization function that automatically instruments the
// tracer
type Tracer struct {
	FsPath string
	InitFn func()
}

func instrumentNode() {
	currNodePath := os.Getenv("NODE_PATH")
	os.Setenv("NODE_PATH", addToString(currNodePath, ":", "/dd_tracer/node/"))

	currNodeOptions := os.Getenv("NODE_OPTIONS")
	os.Setenv("NODE_OPTIONS", addToString(currNodeOptions, " ", "--require dd-trace/init"))
}

func instrumentJava() {
	currJavaToolOptions := os.Getenv("JAVA_TOOL_OPTIONS")
	os.Setenv("JAVA_TOOL_OPTIONS", addToString(currJavaToolOptions, " ", "-javaagent:/dd_tracer/java/dd-java-agent.jar"))
}

func instrumentDotnet() {
	os.Setenv("CORECLR_ENABLE_PROFILING", "1")
	os.Setenv("CORECLR_PROFILER", "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}")
	os.Setenv("CORECLR_PROFILER_PATH", "/dd_tracer/dotnet/Datadog.Trace.ClrProfiler.Native.so")
	os.Setenv("DD_DOTNET_TRACER_HOME", "/dd_tracer/dotnet/")
}

func instrumentPython() {
	os.Setenv("PYTHONPATH", addToString(os.Getenv("PYTHONPATH"), ":", "/dd_tracer/python/"))
}

// AutoInstrumentTracer searches the filesystem for a trace library, and
// automatically sets the correct environment variables.
func autoInstrumentTracer(fs afero.Fs) {
	tracers := []Tracer{
		{"/dd_tracer/node/", instrumentNode},
		{"/dd_tracer/java/", instrumentJava},
		{"/dd_tracer/dotnet/", instrumentDotnet},
		{"/dd_tracer/python/", instrumentPython},
	}

	for _, tracer := range tracers {
		if ok, err := dirExists(fs, tracer.FsPath); ok {
			log.Debugf("Found %v, automatically instrumenting tracer", tracer.FsPath)
			os.Setenv("DD_TRACE_PROPAGATION_STYLE", "datadog")
			tracer.InitFn()
			return
		} else if err != nil {
			log.Debug("Error checking if directory exists: %v", err)
		}
	}
}

func dirExists(fs afero.Fs, path string) (bool, error) {
	_, err := fs.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func addToString(path string, separator string, token string) string {
	if path == "" {
		return token
	}

	return path + separator + token
}
