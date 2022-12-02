// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	pythonSSLServerFormat = `import http.server, ssl, sys

YES = ('true', '1', 't', 'y', 'yes')

class RequestHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    daemon_threads = True

    def do_GET(self):
        status_code = int(self.path.split("/")[1])
        self.send_response(status_code)
        self.send_header('Content-type', 'application/octet-stream')
        self.send_header('Content-Length', '0')
        self.send_header('Connection', 'keep-alive')
        self.end_headers()

server_address = ('%s', %s)
httpd = http.server.HTTPServer(server_address, RequestHandler)

if len(sys.argv) >= 2 and sys.argv[1].lower() in YES:
    httpd.socket = ssl.wrap_socket(httpd.socket,
                                   server_side=True,
                                   certfile='%s',
                                   keyfile='%s',
                                   ssl_version=ssl.PROTOCOL_TLS)
try:
    httpd.serve_forever()
finally:
    httpd.shutdown()
`
)

func HTTPPythonServer(t *testing.T, addr string, options Options) (func(), error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	curDir, _ := CurDir()
	crtPath := filepath.Join(curDir, "testdata/cert.pem.0")
	keyPath := filepath.Join(curDir, "testdata/server.key")
	pythonSSLServer := fmt.Sprintf(pythonSSLServerFormat, host, port, crtPath, keyPath)
	scriptFile, err := writeTempFile("python_openssl_script", pythonSSLServer)
	require.NoError(t, err)
	defer scriptFile.Close()

	cmd := exec.Command("python3", scriptFile.Name(), strconv.FormatBool(options.EnableTLS))
	go require.NoError(t, cmd.Start())

	// Waiting for the server to be ready
	portCtx, cancelPortCtx := context.WithDeadline(context.Background(), time.Now().Add(time.Second*5))
	rawConnect(portCtx, t, host, port)
	cancelPortCtx()

	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}, nil
}

func writeTempFile(pattern string, content string) (*os.File, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return nil, err
	}

	return f, nil
}

func rawConnect(ctx context.Context, t *testing.T, host string, port string) {
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("failed connecting to %s:%s", host, port)
		default:
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), time.Second)
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
