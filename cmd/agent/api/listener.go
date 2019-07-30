// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package api

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// getListener returns a listening connection
func getListener() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("%v:%v", config.Datadog.GetString("cmd_host"), config.Datadog.GetInt("cmd_port")))
}
