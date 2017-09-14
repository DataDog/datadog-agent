// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package api

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	"net"
)

// getListener returns a listening connection
func getListener() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")))
}
