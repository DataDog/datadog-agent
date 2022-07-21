// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
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
)

// Options wraps all configurable params for the HTTPServer
type Options struct {
	EnableTLS        bool
	EnableKeepAlives bool
}

// HTTPServer spins up a HTTP test server that returns the status code included in the URL
// Example:
// * GET /200/foo returns a 200 status code;
// * PUT /404/bar returns a 404 status code;
// Optional TLS support using a self-signed certificate can be enabled trough the `enableTLS` argument
// nolint
func HTTPServer(t *testing.T, addr string, options Options) func() {
	handler := func(w http.ResponseWriter, req *http.Request) {
		statusCode := StatusFromPath(req.URL.Path)
		io.Copy(ioutil.Discard, req.Body)
		w.WriteHeader(statusCode)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      http.HandlerFunc(handler),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}

	listenFn := func() { _ = srv.ListenAndServe() }

	// If certPath is set we enabled TLS
	if options.EnableTLS {
		curDir, _ := curDir()
		crtPath := filepath.Join(curDir, "testdata/cert.pem.0")
		keyPath := filepath.Join(curDir, "testdata/server.key")
		listenFn = func() { _ = srv.ListenAndServeTLS(crtPath, keyPath) }
	}

	go listenFn()
	srv.SetKeepAlivesEnabled(options.EnableKeepAlives)

	// best effort to block until the server is ready (up to 5 seconds)
	// note that `DialTimeout` will return immmeditely if the listen socket is not yet
	// ready, which is why it is called in a loop
	timeout := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(timeout) {
			break
		}
	}

	return func() { srv.Shutdown(context.Background()) }
}

var pathParser = regexp.MustCompile(`/(\d{3})/.+`)

// StatusFromPath returns the status code present in the first segment of the request path
func StatusFromPath(path string) (status int) {
	matches := pathParser.FindStringSubmatch(path)
	if len(matches) == 2 {
		status, _ = strconv.Atoi(matches[1])
	}

	return
}

func curDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
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
