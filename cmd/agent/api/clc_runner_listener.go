// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package api

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
	podIP := config.Datadog.GetString("clc_runner_host")
	if util.IsForbidden(podIP) {
		return nil, fmt.Errorf("Invalid cluster check runner host: %s, must be set to the Pod IP", podIP)
	}
	return net.Listen("tcp", fmt.Sprintf("%v:%v", podIP, config.Datadog.GetInt("clc_runner_port")))
}
