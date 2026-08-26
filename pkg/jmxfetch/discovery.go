// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build jmx

package jmxfetch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// discoveryTimeout is the maximum time a one-shot discovery subprocess can run.
const discoveryTimeout = 60 * time.Second

// RunDiscovery starts a JMXFetch one-shot subprocess in "discover" mode to
// probe a JMX endpoint and return a verified config. The subprocess connects
// to candidate JMX ports, inspects MBeans, runs one collection iteration to
// verify metrics flow, and outputs the discovered config as JSON on stdout.
//
// Returns the config JSON string from stdout, or an error if the subprocess
// fails, times out, or exits with a non-zero code.
func RunDiscovery(integrationName, serviceJSON string) (string, error) {
	classpath := filepath.Join(defaultpaths.GetDistPath(), "jmx", jmxJarName)

	// Add global custom jars
	globalCustomJars := pkgconfigsetup.Datadog().GetStringSlice("jmx_custom_jars")
	if len(globalCustomJars) > 0 {
		classpath = fmt.Sprintf("%s%s%s", strings.Join(globalCustomJars, string(filepath.ListSeparator)), string(filepath.ListSeparator), classpath)
	}

	// Build JVM options (simplified — no container support, no IPC)
	javaOptions := jmxAllowAttachSelf
	javaOptions += defaultJvmMaxMemoryAllocation
	javaOptions += defaultJvmInitialMemoryAllocation

	// Build temp dir option (same as Start())
	if !strings.Contains(javaOptions, "java.io.tmpdir") {
		javaTmpDir := filepath.Join(pkgconfigsetup.Datadog().GetString("run_path"), "jmxfetch")
		javaOptions += " -Djava.io.tmpdir=" + javaTmpDir
	}

	subprocessArgs := []string{}
	subprocessArgs = append(subprocessArgs, strings.Fields(javaOptions)...)
	subprocessArgs = append(subprocessArgs,
		"-classpath", classpath,
		jmxMainClass,
		"--reporter", "console",
		"--log_level", "INFO",
		"discover",
		"--integration", integrationName,
		"--service_json", serviceJSON,
	)

	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, defaultJavaBinPath, subprocessArgs...)

	// Set environment
	cmd.Env = os.Environ()

	// Append JAVA_TOOL_OPTIONS if configured
	javaToolOptions := pkgconfigsetup.Datadog().GetString("jmx_java_tool_options")
	if len(javaToolOptions) > 0 {
		cmd.Env = append(cmd.Env, "JAVA_TOOL_OPTIONS="+javaToolOptions)
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Debugf("JMX discovery: running subprocess: %s %v", defaultJavaBinPath, subprocessArgs)

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		log.Warnf("JMX discovery: subprocess timed out after %v for integration %s", discoveryTimeout, integrationName)
		return "", fmt.Errorf("jmx discovery subprocess timed out after %v", discoveryTimeout)
	}
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		log.Debugf("JMX discovery: subprocess failed for integration %s: %v, stderr: %s", integrationName, err, stderrStr)
		return "", fmt.Errorf("jmx discovery subprocess failed: %w", err)
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		log.Debugf("JMX discovery: subprocess produced no output for integration %s", integrationName)
		return "", fmt.Errorf("jmx discovery subprocess produced no output")
	}

	if result == "[]" {
		log.Debugf("JMX discovery: subprocess found no valid JMX config for integration %s", integrationName)
		return "", fmt.Errorf("no valid JMX config found for integration %s", integrationName)
	}

	log.Infof("JMX discovery: subprocess succeeded for integration %s, result length: %d", integrationName, len(result))
	return result, nil
}
