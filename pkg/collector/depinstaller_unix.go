// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultDepInstallerSocket = "/opt/datadog-packages/run/agent-deps.sock"
	depInstallTimeout         = 30 * time.Second
)

type depInstallerClient struct {
	client *http.Client
}

// newDepInstallerClient returns a client for the installer daemon's restricted
// dependency endpoint over its unix socket, or nil when the feature is not
// usable.
func newDepInstallerClient() DepInstaller {
	socketPath := pkgconfigsetup.Datadog().GetString("on_demand_python_deps.installer_socket_path")
	if socketPath == "" {
		socketPath = defaultDepInstallerSocket
	}
	return &depInstallerClient{
		client: &http.Client{
			Timeout: depInstallTimeout,
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
	}
}

func (c *depInstallerClient) post(pkg, action, version string) error {
	var body io.Reader
	if version != "" {
		data, err := json.Marshal(map[string]string{"version": version})
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://installer/%s/%s", pkg, action), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var response struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("%s", response.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// InstallDepPackage installs the dependency package.
func (c *depInstallerClient) InstallDepPackage(pkg, version string) error {
	log.Infof("Requesting installation of dependency package %s version %s", pkg, version)
	return c.post(pkg, "install", version)
}

// RemoveDepPackage removes the dependency package.
func (c *depInstallerClient) RemoveDepPackage(pkg string) error {
	log.Infof("Requesting removal of dependency package %s", pkg)
	return c.post(pkg, "remove", "")
}

var _ DepInstaller = (*depInstallerClient)(nil)
