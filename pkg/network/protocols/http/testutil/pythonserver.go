// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package testutil provides utilities for testing the HTTP protocol.
package testutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

const (
	// Certificate paths for container environment
	containerCertPath = "/v/cert.pem.0"
	containerKeyPath  = "/v/server.key"

	pythonSSLServerFormat = `import http.server, ssl, sys

YES = ('true', '1', 't', 'y', 'yes')

class RequestHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    daemon_threads = True

    def do_GET(self):
        path = self.path
        if self.path.startswith("/status"):
            path = self.path.split("/status")[1]
        status_code = int(path.split("/")[1])
        self.send_response(status_code)
        self.send_header('Content-type', 'application/octet-stream')
        self.send_header('Content-Length', '0')
        self.send_header('Connection', 'keep-alive')
        self.end_headers()

server_address = ('%s', %s)
httpd = http.server.HTTPServer(server_address, RequestHandler)

if len(sys.argv) >= 2 and sys.argv[1].lower() in YES:
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(certfile='%s', keyfile='%s')
    httpd.socket = context.wrap_socket(httpd.socket, server_side=True)
try:
    print(f"Server running at https://{server_address[0]}:{server_address[1]}/")
    httpd.serve_forever()
finally:
    httpd.shutdown()
`
)

// HTTPPythonServer launches an HTTP python server.
func HTTPPythonServer(t *testing.T, addr string, options Options) *exec.Cmd {
	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	curDir, _ := CurDir()
	crtPath := filepath.Join(curDir, "testdata/cert.pem.0")
	if options.CertPath != "" {
		crtPath = options.CertPath
	}
	keyPath := filepath.Join(curDir, "testdata/server.key")
	if options.KeyPath != "" {
		keyPath = options.KeyPath
	}
	pythonSSLServer := fmt.Sprintf(pythonSSLServerFormat, host, port, crtPath, keyPath)
	scriptFile, err := writeTempFile("python_openssl_script", pythonSSLServer)
	require.NoError(t, err)

	cmd := exec.Command("python3", scriptFile.Name(), strconv.FormatBool(options.EnableTLS))

	go require.NoError(t, cmd.Start())

	// Waiting for the server to be ready
	portCtx, cancelPortCtx := context.WithDeadline(context.Background(), time.Now().Add(time.Second*5))
	rawConnect(portCtx, t, host, port)
	cancelPortCtx()

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	return cmd
}

func writeTempFile(pattern, content string) (*os.File, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Warnf("failed closing file: %v\n", err)
		}
	}()

	if _, err := f.WriteString(content); err != nil {
		return nil, err
	}

	return f, nil
}

func rawConnect(ctx context.Context, t *testing.T, host, port string) {
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("failed connecting to %s:%s", host, port)
		default:
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), time.Millisecond*100)
			if err != nil {
				continue
			}
			if conn != nil {
				conn.Close()
				return
			}
		}
	}
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func linkFile(t *testing.T, src, dst string) error {
	t.Helper()
	_ = os.Remove(dst)
	if err := copyFile(src, dst); err != nil {
		return err
	}
	t.Cleanup(func() { os.Remove(dst) })
	return nil
}

// HTTPPythonServerContainer launches an HTTPs server written in Python inside a container.
func HTTPPythonServerContainer(t *testing.T, serverPort string) error {
	t.Helper()
	dir, _ := CurDir()

	// Create Python script using existing format - reference original certificate files directly
	pythonSSLServer := fmt.Sprintf(pythonSSLServerFormat, "0.0.0.0", "4141", containerCertPath, containerKeyPath)
	scriptFile, err := writeTempFile("python_container_script", pythonSSLServer)
	require.NoError(t, err)

	// Copy script to testdata directory so it can be mounted
	if err := linkFile(t, scriptFile.Name(), dir+"/testdata/server.py"); err != nil {
		return err
	}

	env := []string{
		"ADDR=0.0.0.0",
		"PORT=" + serverPort,
		"CERTS_DIR=/v/certs",
		"TESTDIR=" + dir + "/testdata",
	}

	scanner, err := globalutils.NewScanner(regexp.MustCompile("Server running at https.*"), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"python-server",
			scanner,
			dockerutils.WithEnv(env),
		),
		path.Join(dir, "testdata", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}

// GetPythonDockerPID returns the PID of the python docker container.
func GetPythonDockerPID() (int64, error) {
	return dockerutils.GetMainPID("python-python-1")
}
