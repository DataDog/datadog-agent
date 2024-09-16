// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package systemprobe is sets up the remote testing environment for system-probe using the Kernel Matrix Testing framework
package systemprobe

import (
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/datadog-agent/test/new-e2e/system-probe/connector/metric"
)

const (
	// bitmap of actions to take for an error
	retryStack = 0x1 // 0b01
	emitMetric = 0x2 // 0b10
	changeAZ   = 0x4 // 0b100

	aria2cMissingStatusErrorStr = "error: wait: remote command exited without exit status or exit signal: running \" aria2c"

	retryCountFile  = "e2e-retry-count"
	errorReasonFile = "e2e-error-reason"
)

type scenarioError int

const (
	libvirtDialError scenarioError = iota
	insufficientCapacityError
	aria2cMissingStatusError
	ec2StateChangeTimeoutError
	ioTimeout
	tcp22ConnectionRefused
	ec2InstanceCreateTimeout
	ddAgentRepoFailure
)

type handledError struct {
	errorType   scenarioError
	errorString string
	metric      string
	action      int
}

var handledErrorsLs = []handledError{
	// Retry if we failed to dial libvirt.
	// Libvirt daemon on the server occasionally drops the connection established
	// by the 'Provider'. If this happens we retry the stack to connect again.
	{
		errorType:   libvirtDialError,
		errorString: "failed to dial libvirt",
		metric:      "failed-to-dial-libvirt",
		action:      retryStack | emitMetric,
	},
	{
		errorType:   insufficientCapacityError,
		errorString: "InsufficientInstanceCapacity",
		metric:      "insufficient-capacity",
		action:      retryStack | emitMetric | changeAZ,
	},
	// Retry when ssh thinks aria2c exited without status. This may happen
	// due to network connectivity issues if ssh keepalive mecahnism fails.
	{
		errorType:   aria2cMissingStatusError,
		errorString: aria2cMissingStatusErrorStr,
		metric:      "aria2c-exit-no-status",
		action:      retryStack | emitMetric,
	},
	{
		errorType:   ec2StateChangeTimeoutError,
		errorString: "timeout while waiting for state to become 'running'",
		metric:      "ec2-timeout-state-change",
		action:      retryStack | emitMetric | changeAZ,
	},
	{
		errorType:   ioTimeout,
		errorString: "i/o timeout",
		metric:      "io-timeout",
		action:      retryStack | emitMetric,
	},
	{
		errorType:   tcp22ConnectionRefused,
		errorString: "failed attempts: dial tcp :22: connect: connection refused",
		metric:      "ssh-connection-refused",
		action:      retryStack | emitMetric,
	},
	{
		errorType:   tcp22ConnectionRefused,
		errorString: "failed attempts: ssh: rejected: connect failed",
		metric:      "ssh-connection-refused",
		action:      retryStack | emitMetric,
	},
	{
		errorType:   ec2InstanceCreateTimeout,
		errorString: "creating EC2 Instance: operation error",
		metric:      "ec2-instance-create-timeout",
		action:      retryStack | emitMetric,
	},
	{
		errorType:   ddAgentRepoFailure,
		errorString: "Failed to update the sources after adding the Datadog repository.",
		metric:      "apt-dd-agent-repo-failure",
		action:      retryStack | emitMetric,
	},
}

type retryHandler struct {
	currentAZ  int
	maxRetries int
	retryDelay time.Duration
	allErrors  []error
	configMap  runner.ConfigMap
	infraEnv   string
}

func errorMetric(errType string) datadogV2.MetricPayload {
	tags := []string{
		fmt.Sprintf("error:%s", errType),
	}
	return datadogV2.MetricPayload{
		Series: []datadogV2.MetricSeries{
			{
				Metric: "datadog.e2e.system_probe.env-setup",
				Type:   datadogV2.METRICINTAKETYPE_COUNT.Ptr(),
				Points: []datadogV2.MetricPoint{
					{
						Timestamp: datadog.PtrInt64(time.Now().Unix()),
						Value:     datadog.PtrFloat64(1),
					},
				},
				Tags: tags,
			},
		},
	}
}

func (r *retryHandler) HandleError(err error, retryCount int) (infra.RetryType, []infra.GetStackOption) {
	r.allErrors = append(r.allErrors, err)

	if retryCount > r.maxRetries {
		log.Printf("environment setup error: %v. Maximum number of retries (%d) exceeded, failing setup.\n", err, r.maxRetries)
		return infra.NoRetry, nil
	}

	var newOpts []infra.GetStackOption
	retry := infra.NoRetry
	errStr := err.Error()
	for _, e := range handledErrorsLs {
		if !strings.Contains(errStr, e.errorString) {
			continue
		}

		if (e.action & changeAZ) != 0 {
			r.currentAZ++
			if az := getAvailabilityZone(r.infraEnv, r.currentAZ); az != "" {
				r.configMap["ddinfra:aws/defaultSubnets"] = auto.ConfigValue{Value: az}
				newOpts = append(newOpts, infra.WithConfigMap(r.configMap))
			}
		}

		if (e.action & emitMetric) != 0 {
			submitError := metric.SubmitExecutionMetric(errorMetric(e.metric))
			if submitError != nil {
				log.Printf("failed to submit environment setup error metrics: %v\n", submitError)
			}

			if storeErr := storeErrorReasonForCITags(e.metric); storeErr != nil {
				log.Printf("failed to store error reason for CI tags: %v\n", storeErr)
			}
		}

		if (e.action & retryStack) != 0 {
			retry = infra.ReUp
		}

		break
	}

	log.Printf("environment setup error. Retry strategy: %s.\n", retry)
	if retry != infra.NoRetry {
		log.Printf("waiting %s before retrying...\n", r.retryDelay)
		time.Sleep(r.retryDelay)
	}

	return retry, newOpts
}

func storeErrorReasonForCITags(reason string) error {
	f, err := os.OpenFile(path.Join(os.Getenv("CI_PROJECT_DIR"), errorReasonFile), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(reason)
	return err
}

func storeNumberOfRetriesForCITags(retries int) error {
	f, err := os.OpenFile(path.Join(os.Getenv("CI_PROJECT_DIR"), retryCountFile), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("%d", retries))
	return err
}

type pulumiError struct {
	command      string
	arch         string
	vmCommand    string
	errorMessage string
	vmName       string
}

var commandRegex = regexp.MustCompile(`^  command:remote:Command \(([^\)]+)\):$`)

func parsePulumiDiagnostics(message string) *pulumiError {
	var perr pulumiError
	lines := strings.Split(message, "\n")
	inDiagnostics := false
	for _, line := range lines {
		if !inDiagnostics {
			if line == "Diagnostics:" {
				// skip until next line
				inDiagnostics = true
			}
			continue
		}

		if len(line) == 0 || line[0] != ' ' {
			// Finished reading diagnostics, break out of the loop
			return &perr
		}

		if perr.command == "" {
			commandMatch := commandRegex.FindStringSubmatch(line)
			if commandMatch != nil {
				perr.command = commandMatch[1]

				perr.arch, perr.vmCommand, perr.vmName = parsePulumiComand(perr.command)
			}
		} else {
			perr.errorMessage += strings.Trim(line, " ") + "\n"
		}
	}

	return nil
}

var archRegex = regexp.MustCompile(`distro_(arm64|x86_64)`)
var vmCmdRegex = regexp.MustCompile(`-cmd-.+-(?:ddvm-\d+-\d+|distro_(?:x86_64|arm64))-(.+)$`)
var vmNameRegex = regexp.MustCompile(`-(?:conn|cmd)-(?:arm64|x86_64)-([^-]+)-`)

func parsePulumiComand(command string) (arch, vmCommand, vmName string) {
	archMatch := archRegex.FindStringSubmatch(command)
	if archMatch != nil {
		arch = archMatch[1]
	}

	vmCmdMatch := vmCmdRegex.FindStringSubmatch(command)
	if vmCmdMatch != nil {
		vmCommand = vmCmdMatch[1]
	}

	vmNameMatch := vmNameRegex.FindStringSubmatch(command)
	if vmNameMatch != nil {
		vmName = vmNameMatch[1]
	}

	return
}
