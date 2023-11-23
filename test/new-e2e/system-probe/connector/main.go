// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package main implements the SSH connector between gitlab runners, metal instances, and micro VMs
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/test/new-e2e/system-probe/connector/metric"
	"github.com/DataDog/datadog-agent/test/new-e2e/system-probe/connector/sshtools"
)

const (
	failConfig  = "config_fail"
	failConnect = "connect_fail"
	failStart   = "start_fail"
	failWait    = "wait_fail"
	success     = "success"
	fail        = "fail"
)

type args struct {
	host                    string
	user                    string
	port                    int
	serverKeepAliveInterval time.Duration
	serverKeepAliveMaxCount int
	sshFilePath             string
	vmCommand               string
}

func readArgs() *args {
	userPtr := flag.String("user", "", "SSH user")
	hostPtr := flag.String("host", "", "Host ip to connect to")
	portPtr := flag.Int("port", 22, "Port for ssh server")
	serverAlivePtr := flag.Int("server-alive-interval", 5, "Interval at which to send keep alive messages")
	serverAliveCountPtr := flag.Int("server-alive-count", 560, "Maximum keep alive messages to send before disconnecting upon no reply")
	sshFilePathPtr := flag.String("ssh-file", "", "Path to private ssh key")
	vmCmd := flag.String("vm-cmd", "", "command to run on VM")

	flag.Parse()

	return &args{
		host:                    *hostPtr,
		user:                    *userPtr,
		port:                    *portPtr,
		serverKeepAliveInterval: time.Duration(*serverAlivePtr) * time.Second,
		serverKeepAliveMaxCount: *serverAliveCountPtr,
		sshFilePath:             *sshFilePathPtr,
		vmCommand:               *vmCmd,
	}
}

type connectorInfo struct {
	// For gitlab runner this will be the job id
	// For metal instance this will be empty
	connectorHost string
	connectorType string
}

func getConnectorInfo() (connectorInfo, error) {
	connectorType := "metal_to_vm"
	if _, ok := os.LookupEnv("GITLAB_CI"); ok {
		connectorType = "gitlab_to_metal"
	}

	return connectorInfo{
		connectorHost: os.Getenv("CI_JOB_ID"),
		connectorType: connectorType,
	}, nil
}

func sshCommunicator(args *args, sshKey []byte) (*sshtools.Communicator, error) {
	config := sshtools.Config{
		Port:                args.port,
		ServerAliveInterval: args.serverKeepAliveInterval,
		ServerAliveCountMax: args.serverKeepAliveMaxCount,
	}
	config, err := config.WithIdentityFileAuth(args.user, sshKey)
	if err != nil {
		return nil, fmt.Errorf("unable to build sshtools config: %w", err)
	}
	config.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	return sshtools.NewCommunicator(args.host, config, sshtools.ContextDialer(&net.Dialer{}), log.New(os.Stdout, "", log.LstdFlags)), nil
}

func main() {
	code := 0
	if err := run(); err != nil {
		log.Print(err)

		code = 1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
			if code < 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func run() (err error) {
	args := readArgs()
	cinfo, err := getConnectorInfo()
	if err != nil {
		return fmt.Errorf("get connector info: %s", err)
	}
	var cmd sshtools.Cmd
	key, err := os.ReadFile(args.sshFilePath)
	if err != nil {
		return fmt.Errorf("read private key: %s", err)
	}

	var failType string
	result := fail
	defer func() {
		if serr := metric.SubmitExecutionMetric(buildMetric(cinfo, failType, result)); serr != nil {
			err = serr
		}
	}()

	communicator, err := sshCommunicator(args, key)
	if err != nil {
		failType = failConfig
		return fmt.Errorf("communicator: %s", err)
	}

	ctx := context.Background()
	if err := communicator.Connect(ctx); err != nil {
		failType = failConnect
		return fmt.Errorf("connect: %s", err)
	}

	if val := os.Getenv("DD_API_KEY"); val != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("DD_API_KEY=%s", val))
	}

	cmd.Command = args.vmCommand
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := communicator.Start(ctx, &cmd); err != nil {
		failType = failStart
		return fmt.Errorf("communicator start: %s", err)
	}

	if err := cmd.Wait(); err != nil {
		var ee *exec.ExitError
		// commands that exit, even with a non-zero exit code are considered success from an SSH PoV
		if errors.As(err, &ee) && ee.Exited() {
			return fmt.Errorf("wait: %s", err)
		}

		failType = failWait
		return fmt.Errorf("wait: %s", err)
	}

	result = success
	return nil
}

func buildMetric(cinfo connectorInfo, failType, result string) datadogV2.MetricPayload {
	tags := []string{
		fmt.Sprintf("result:%s", result),
		fmt.Sprintf("connection_type:%s", cinfo.connectorType),
	}
	if failType != "" {
		tags = append(tags, fmt.Sprintf("error:%s", failType))
	}
	return datadogV2.MetricPayload{
		Series: []datadogV2.MetricSeries{
			{
				Metric: "datadog.e2e.system_probe.ssh.commands",
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
