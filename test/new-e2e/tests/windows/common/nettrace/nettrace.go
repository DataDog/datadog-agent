// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package nettrace uses netsh trace to capture network traffic
package nettrace

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// NetTrace represents a network trace using netsh trace
type NetTrace struct {
	host   *components.RemoteHost
	params *params
}

// Option is a function that modifies the Params
type Option func(*params)

// New creates a new network trace using netsh trace
//
// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-server-2012-r2-and-2012/jj129382(v=ws.11)
func New(host *components.RemoteHost, opts ...Option) (*NetTrace, error) {
	params := newParams(opts...)

	if params.tracefile == "" {
		tempFile, err := windowsCommon.GetTemporaryFile(host)
		if err != nil {
			return nil, err
		}
		params.tracefile = tempFile
	}

	nt := &NetTrace{
		host:   host,
		params: params,
	}

	return nt, nil
}

// Start starts the network trace
func (nt *NetTrace) Start() error {
	// always overwrite the trace file so GetTemporaryFile() can be used
	cmd := fmt.Sprintf("netsh trace start capture=yes overwrite=yes %s", nt.params.ToArgs())
	fmt.Printf("Starting trace with command: %s\n", cmd)
	_, err := nt.host.Execute(cmd)
	return err
}

// Stop stops the network trace
func (nt *NetTrace) Stop() error {
	cmd := "netsh trace stop"
	_, err := nt.host.Execute(cmd)
	return err
}

// Cleanup stops the network trace and removes the trace file
func (nt *NetTrace) Cleanup() error {
	err := nt.Stop()
	if err != nil {
		// ignore error if there is no trace session
		if !strings.Contains(err.Error(), "There is no trace session currently in progress.") {
			return err
		}
	}

	// remove the trace file
	path := nt.params.tracefile
	exists, err := nt.host.FileExists(path)
	if err != nil {
		return fmt.Errorf("failed to check if snapshot exists %s: %w", path, err)
	}
	if !exists {
		return nil
	}
	err = nt.host.Remove(path)
	if err != nil {
		return fmt.Errorf("failed to remove snapshot %s: %w", path, err)
	}
	return nil
}

// TraceFile returns the path to the trace file
func (nt *NetTrace) TraceFile() string {
	return nt.params.tracefile
}

// GetPCAPNG converts the ETL file to a pcapng file and downloads it to the local machine
func GetPCAPNG(nt *NetTrace, destination string) error {
	// temporary file to store the pcapng file on remote host
	tmp, err := windowsCommon.GetTemporaryFile(nt.host)
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer nt.host.Remove(tmp)
	// convert the ETL file to pcapng
	err = etl2pcapng(nt.host, nt.TraceFile(), tmp)
	if err != nil {
		return err
	}
	// download the pcapng file
	err = nt.host.GetFile(tmp, destination)
	if err != nil {
		return err
	}
	return nil
}

// etl2pcapng converts an ETL file to a pcapng file
//
// https://github.com/microsoft/etl2pcapng
func etl2pcapng(host *components.RemoteHost, etlFile string, pcapngFile string) error {
	destination, err := fetchetl2pcapng(host)
	if err != nil {
		return err
	}

	_, err = host.Execute(fmt.Sprintf(`& '%s' '%s' '%s'`, destination, etlFile, pcapngFile))
	if err != nil {
		return err
	}

	return nil
}

func fetchetl2pcapng(host *components.RemoteHost) (string, error) {
	// TODO: Use the E2E tool cache once it is available
	url := `https://github.com/microsoft/etl2pcapng/releases/download/v1.11.0/etl2pcapng.exe`
	destination := `C:\Windows\Temp\etl2pcapng.exe`
	exits, err := host.FileExists(destination)
	if err != nil {
		return "", err
	}
	if !exits {
		err := windowsCommon.DownloadFile(host, url, destination)
		if err != nil {
			return "", err
		}
	}
	return destination, nil
}
