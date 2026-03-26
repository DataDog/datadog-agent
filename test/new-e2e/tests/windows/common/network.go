// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"

	"github.com/cenkalti/backoff/v5"
)

// BoundPort represents a port that is bound to a process
type BoundPort struct {
	localAddress string
	localPort    int
	processName  string
	pid          int
}

// LocalAddress returns the local address of the bound port
func (b *BoundPort) LocalAddress() string {
	return b.localAddress
}

// LocalPort returns the local port of the bound port
func (b *BoundPort) LocalPort() int {
	return b.localPort
}

// Transport returns the transport protocol of the bound port
func (b *BoundPort) Transport() string {
	// TODO: We currently only collect listening TCP sockets. We could likely get UDP by querying `Get-NetUDPEndpoint`.
	return "tcp"
}

// Process returns the process name of the bound port
func (b *BoundPort) Process() string {
	return b.processName
}

// PID returns the PID of the bound port
func (b *BoundPort) PID() int {
	return b.pid
}

// ListBoundPorts returns a list of bound ports
func ListBoundPorts(host *components.RemoteHost) ([]*BoundPort, error) {
	out, err := host.Execute(`Get-NetTCPConnection -State Listen | Foreach-Object {
		@{
			LocalAddress=$_.LocalAddress
			LocalPort = $_.LocalPort
			Process = (Get-Process -Id $_.OwningProcess).Name
			PID = $_.OwningProcess
		}} | ConvertTo-JSON`)
	if err != nil {
		return nil, err
	}

	// unmarshal out as JSON
	var ports []map[string]any
	err = json.Unmarshal([]byte(out), &ports)
	if err != nil {
		return nil, err
	}

	// process JSON to BoundPort
	boundPorts := make([]*BoundPort, 0, len(ports))
	for _, port := range ports {
		boundPorts = append(boundPorts, &BoundPort{
			localAddress: port["LocalAddress"].(string),
			localPort:    int(port["LocalPort"].(float64)),
			processName:  port["Process"].(string),
			pid:          int(port["PID"].(float64)),
		})
	}

	return boundPorts, nil
}

// PutOrDownloadFile creates a file on the VM from a file/http URL
//
// If the URL is a local file, it will be uploaded to the VM.
// If the URL is a remote file, it will be downloaded from the VM
func PutOrDownloadFile(host *components.RemoteHost, url string, destination string) error {
	// no retry
	return PutOrDownloadFileWithRetry(host, url, destination, backoff.WithBackOff(&backoff.StopBackOff{}))
}

// PutOrDownloadFileWithRetry is similar to PutOrDownloadFile but retries on download failure,
// local file copy is not retried.
func PutOrDownloadFileWithRetry(host *components.RemoteHost, url string, destination string, opts ...backoff.RetryOption) error {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		_, err := backoff.Retry(context.Background(), func() (any, error) {
			return nil, DownloadFile(host, url, destination)
			// TODO: it would be neat to only retry on web related errors but
			//       we don't have a way to distinguish them since DownloadFile
			//       throws a WebException for non web related errors such as
			//       filename is null or Empty.
			//       https://learn.microsoft.com/en-us/dotnet/api/system.net.webclient.downloadfile
			//       example error: Exception calling "DownloadFile" with "2" argument(s): "The remote server returned an error: (503)
		}, opts...)
		if err != nil {
			return err
		}
		return nil
	}

	if after, ok := strings.CutPrefix(url, "file://"); ok {
		// URL is a local file
		localPath := after
		host.CopyFile(localPath, destination)
		return nil
	}

	// just assume it's a local file
	host.CopyFile(url, destination)
	return nil
}

// DownloadFile downloads a file on the VM from a http/https URL
func DownloadFile(host *components.RemoteHost, url string, destination string) error {
	// Note: Avoid using Invoke-WebRequest to download files non-interactively,
	// its progress bar behavior significantly increases download time.
	// https://github.com/PowerShell/PowerShell/issues/2138
	// Enable TLS 1.2 for older Windows (e.g. Server 2016) so HTTPS downloads succeed.
	_, err := host.Execute(fmt.Sprintf("[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; (New-Object Net.WebClient).DownloadFile('%s','%s')", url, destination))
	if err != nil {
		return err
	}

	return nil
}
