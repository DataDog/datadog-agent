// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package traceroute

import (
	"net/http"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

var getSysProbeClient = funcs.MemoizeNoError(func() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: sysprobeclient.DialContextFunc(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
		},
	}
})
