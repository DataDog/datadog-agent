// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fallbackHostnameFunc specifies the function to use for obtaining the hostname
// when it can not be obtained by any other means. It is replaced in tests.
var fallbackHostnameFunc = os.Hostname

func hostname(c *config.AgentConfig) error {
	// no user-set hostname, try to acquire
	if err := acquireHostname(c); err != nil {
		log.Infof("Could not get hostname via gRPC: %v. Falling back to other methods.", err)
		if err := acquireHostnameFallback(c); err != nil {
			return err
		}
	}
	return nil
}

// acquireHostname attempts to acquire a hostname for the trace-agent by connecting to the core agent's
// gRPC endpoints. If it fails, it will return an error.
func acquireHostname(c *config.AgentConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ipcAddress, err := coreconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	client, err := grpc.GetDDAgentClient(ctx, ipcAddress, coreconfig.GetIPCPort())
	if err != nil {
		return err
	}
	reply, err := client.GetHostname(ctx, &pbgo.HostnameRequest{})
	if err != nil {
		return err
	}
	if c.HasFeature("disable_empty_hostname") && reply.Hostname == "" {
		log.Infof("Acquired empty hostname from gRPC but it's disallowed.")
		return errors.New("empty hostname disallowed")
	}
	c.Hostname = reply.Hostname
	log.Infof("Acquired hostname from gRPC: %s", c.Hostname)
	return nil
}

// acquireHostnameFallback attempts to acquire a hostname for this configuration. It
// tries to shell out to the infrastructure agent for this, if DD_AGENT_BIN is
// set, otherwise falling back to os.Hostname.
func acquireHostnameFallback(c *config.AgentConfig) error {
	var out bytes.Buffer
	cmd := exec.Command(c.DDAgentBin, "hostname")
	cmd.Env = append(os.Environ(), cmd.Env...) // needed for Windows
	cmd.Stdout = &out
	err := cmd.Run()
	c.Hostname = strings.TrimSpace(out.String())
	if emptyDisallowed := c.HasFeature("disable_empty_hostname") && c.Hostname == ""; err != nil || emptyDisallowed {
		if emptyDisallowed {
			log.Infof("Core agent returned empty hostname but is disallowed by disable_empty_hostname feature flag. Falling back to os.Hostname.")
		}
		// There was either an error retrieving the hostname from the core agent, or
		// it was empty and its disallowed by the disable_empty_hostname feature flag.
		host, err2 := fallbackHostnameFunc()
		if err2 != nil {
			return fmt.Errorf("couldn't get hostname from agent (%q), nor from OS (%q). Try specifying it by means of config or the DD_HOSTNAME env var", err, err2)
		}
		if emptyDisallowed && host == "" {
			return errors.New("empty hostname disallowed")
		}
		c.Hostname = host
		log.Infof("Acquired hostname from OS: %q. Core agent was unreachable at %q: %v.", c.Hostname, c.DDAgentBin, err)
		return nil
	}
	log.Infof("Acquired hostname from core agent (%s): %q.", c.DDAgentBin, c.Hostname)
	return nil
}
