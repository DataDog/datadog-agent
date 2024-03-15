// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	sysctl "github.com/lorenzosaino/go-sysctl"
)

// Options wraps all configurable params for the HTTPServer
type Options struct {
	// If TLS is enabled, allows to upgrade the connections to http/2.
	EnableHTTP2        bool
	EnableTLS          bool
	EnableKeepAlive    bool
	EnableTCPTimestamp *bool
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	SlowResponse       time.Duration
}

func isNetIPV4TCPTimestampEnabled(t *testing.T) bool {
	oldTCPTS, err := sysctl.Get("net.ipv4.tcp_timestamps")
	if err != nil {
		t.Logf("can't get TCP timestamp %s", err)
		return false
	}
	return oldTCPTS == "1"
}

func setNetIPV4TCPTimestamp(t *testing.T, enable bool) {
	if os.Geteuid() != 0 {
		if isNetIPV4TCPTimestampEnabled(t) != enable {
			t.Skip("skipping as we don't have enough permission to change net.ipv4.tcp_timestamps")
		}
		return
	}

	tcpTimestampStr := "0"
	if enable {
		tcpTimestampStr = "1"
	}

	if err := sysctl.Set("net.ipv4.tcp_timestamps", tcpTimestampStr); err != nil {
		t.Errorf("can't set old value of TCP timestamp %s", err)
	}
}

// SetupNetIPV4TCPTimestamp sets the net.ipv4.tcp_timestamps to the provided value.
func SetupNetIPV4TCPTimestamp(t *testing.T, enable bool) {
	oldTCPTS := isNetIPV4TCPTimestampEnabled(t)
	setNetIPV4TCPTimestamp(t, enable)
	t.Cleanup(func() { setNetIPV4TCPTimestamp(t, oldTCPTS) })
}

// HTTPServer spins up a HTTP test server that returns the status code included in the URL
// Example:
// * GET /200/foo returns a 200 status code;
// * PUT /404/bar returns a 404 status code;
// Optional TLS support using a self-signed certificate can be enabled trough the `enableTLS` argument
// nolint
func HTTPServer(t *testing.T, addr string, options Options) func() {
	handler := func(w http.ResponseWriter, req *http.Request) {
		if options.SlowResponse != 0 {
			time.Sleep(options.SlowResponse)
		}
		statusCode := StatusFromPath(req.URL.Path)
		if statusCode == 0 {
			t.Errorf("wrong request format %s", req.URL.Path)
		} else {
			w.WriteHeader(int(statusCode))
		}

		defer req.Body.Close()
		io.Copy(w, req.Body)
	}
	/* Save and recover TCP timestamp option */
	if options.EnableTCPTimestamp != nil {
		SetupNetIPV4TCPTimestamp(t, *options.EnableTCPTimestamp)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      http.HandlerFunc(handler),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	if !options.EnableHTTP2 {
		// Disabling http2
		srv.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	}
	srv.SetKeepAlivesEnabled(options.EnableKeepAlive)

	listenFn := func() error {
		ln, err := net.Listen("tcp", srv.Addr)
		if err == nil {
			go func() { _ = srv.Serve(ln) }()
		}
		return err
	}
	if options.ReadTimeout != 0 {
		srv.ReadTimeout = options.ReadTimeout
	}

	if options.WriteTimeout != 0 {
		srv.WriteTimeout = options.WriteTimeout
	}

	// If certPath is set we enabled TLS
	if options.EnableTLS {
		curDir, _ := CurDir()
		crtPath := filepath.Join(curDir, "testdata/cert.pem.0")
		keyPath := filepath.Join(curDir, "testdata/server.key")
		listenFn = func() error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err == nil {
				go func() { _ = srv.ServeTLS(ln, crtPath, keyPath) }()
			}
			return err
		}
	}

	if err := listenFn(); err != nil {
		t.Fatalf("server listen: %s", err)
	}
	return func() {
		srv.Shutdown(context.Background())
	}
}

var pathParser1 = regexp.MustCompile(`/(\d{3})/.+`)
var pathParser2 = regexp.MustCompile(`/status/(\d{3})$`)

// StatusFromPath returns the status code present in the first segment of the request path
func StatusFromPath(path string) uint16 {
	matches := pathParser1.FindStringSubmatch(path)
	if len(matches) == 2 {
		status, _ := strconv.Atoi(matches[1])
		return uint16(status)
	}

	matches = pathParser2.FindStringSubmatch(path)
	if len(matches) == 2 {
		status, _ := strconv.Atoi(matches[1])
		return uint16(status)
	}
	return 0
}

// GetCertsPaths returns the absolute paths to the certs located in the testdata
// directory, so they can be used in test throughout the project
func GetCertsPaths() (string, string, error) {
	curDir, err := CurDir()
	if err != nil {
		return "", "", err
	}

	return filepath.Join(curDir, "testdata/cert.pem.0"), filepath.Join(curDir, "testdata/server.key"), nil
}

// CurDir returns the current directory of the caller.
func CurDir() (string, error) {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("unable to get current file build path")
	}

	buildDir := filepath.Dir(file)

	// build relative path from base of repo
	buildRoot := rootDir(buildDir)
	relPath, err := filepath.Rel(buildRoot, buildDir)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	curRoot := rootDir(cwd)

	return filepath.Join(curRoot, relPath), nil
}

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}
