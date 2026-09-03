// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"net/http"

	"github.com/Microsoft/go-winio"

	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	namedPipePath = "\\\\.\\pipe\\DD_INSTALLER"
)

// NewLocalAPI returns a new LocalAPI.
func NewLocalAPI(daemon Daemon) (localapi.Component, error) {
	// Prevent daemon from running in insecure directories
	err := paths.IsInstallerDataDirSecure()
	if err != nil {
		return nil, err
	}
	listener, err := winio.ListenPipe(namedPipePath, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)",
		MessageMode:        false,
	})
	if err != nil {
		return nil, err
	}
	return &localAPIImpl{
		server:   &http.Server{},
		listener: listener,
		daemon:   daemon,
	}, nil
}
