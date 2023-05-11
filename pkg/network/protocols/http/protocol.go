// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type httpProtocol struct {
	telemetry  *Telemetry
	statkeeper *HttpStatKeeper
}

func newHttpProtocol(c *config.Config) (protocols.Protocol, error) {
	if !c.EnableHTTPMonitoring {
		return nil, nil
	}

	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("couldn't determine current kernel version: %w", err)
	}

	if kversion < MinimumKernelVersion {
		return nil, fmt.Errorf("http feature not available on pre %s kernels", MinimumKernelVersion.String())

	}

	telemetry := NewTelemetry()
	statkeeper := NewHTTPStatkeeper(c, telemetry)

	return &httpProtocol{
		telemetry:  telemetry,
		statkeeper: statkeeper,
	}, nil
}

func init() {
	protocols.RegisterProtocolFactory(protocols.Http, newHttpProtocol)

	log.Debug("[USM] Registered HTTP protocol factory")
}
