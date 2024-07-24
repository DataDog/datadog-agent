// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package clcrunnerapi implements the clc runner IPC api. Using HTTP
calls, the cluster Agent collects stats to optimize the cluster level checks dispatching.
*/
package clcrunnerapi

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// getCLCRunnerListener returns a listening connection for the cluster level check runner server
// The server must only listen on the cluster check runner pod ip
// The cluster check runner Agent won't start if the server host is not configured
func getCLCRunnerListener() (net.Listener, error) {
	podIP := config.Datadog().GetString("clc_runner_host")
	// This is not a security feature
	// util.IsForbidden only helps to avoid unnecessarily permissive server config
	if util.IsForbidden(podIP) {
		// The server must only listen on the Pod IP
		return nil, fmt.Errorf("Invalid cluster check runner host: %s, must be set to the Pod IP", podIP)
	}
	if util.IsIPv6(podIP) {
		// IPv6 addresses must be formatted [ip]:port
		podIP = fmt.Sprintf("[%s]", podIP)
	}
	return net.Listen("tcp", fmt.Sprintf("%v:%v", podIP, config.Datadog().GetInt("clc_runner_port")))
}
