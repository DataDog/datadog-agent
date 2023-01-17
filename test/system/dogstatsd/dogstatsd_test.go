// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/require"
)

var (
	dogstatsdBin = os.Getenv("DOGSTATSD_BIN")
)

type dogstatsdTest struct {
	tmpDir       string
	ctx          context.Context
	cancel       context.CancelFunc
	requests     []string
	ts           *httptest.Server
	conn         net.Conn
	requestReady chan bool
	m            sync.Mutex
}

func setupDogstatsd(t *testing.T) *dogstatsdTest {
	d := &dogstatsdTest{
		requestReady: make(chan bool, 10),
	}
	d.setup(t)
	return d
}

func (d *dogstatsdTest) setup(t *testing.T) {
	require.NotEqual(t, dogstatsdBin, "", "dogstatsd binary path not set in env")

	// start fake backend
	d.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err, "Could not read request Body")

		d.m.Lock()
		d.requests = append(d.requests, string(body))
		d.m.Unlock()
		fmt.Fprintln(w, "OK")
		d.requestReady <- true
	}))

	// create temp dir
	d.tmpDir = t.TempDir()

	// write temp conf
	content := []byte("dd_url: " + d.ts.URL + "\napi_key: dummy\n")
	tmpConf := filepath.Join(d.tmpDir, "datadog.yaml")
	err := os.WriteFile(tmpConf, content, 0666)
	require.NoError(t, err, "Could not write temp conf")

	// start dogstatsd
	d.ctx, d.cancel = context.WithCancel(context.Background())
	e := exec.CommandContext(d.ctx, dogstatsdBin, "start", "-f", tmpConf)
	stdout, err := e.StdoutPipe()
	require.NoError(t, err, "Could get StdoutPipe from command")
	go func() {
		in := bufio.NewScanner(stdout)

		for in.Scan() {
			log.Infof(in.Text())
		}
		if err := in.Err(); err != nil {
			log.Errorf("error: %s", err)
		}
	}()

	go e.Run()
	// give it a second to start
	time.Sleep(1 * time.Second)

	// prepare UDP conn
	conn, err := net.Dial("udp", "127.0.0.1:8125")
	require.Nil(t, err, "could not connect to dogstatsd UDP port")
	d.conn = conn
}

func (d *dogstatsdTest) teardown() {
	// close UDP conn
	d.conn.Close()

	// stop dogstatsd
	d.cancel()

	// stop fake backend
	d.ts.Close()
}

func (d *dogstatsdTest) getRequests() []string {
	d.m.Lock()
	defer d.m.Unlock()

	requests := d.requests
	d.requests = nil
	return requests
}

// Helpers

func (d *dogstatsdTest) sendUDP(msg string) {
	d.conn.Write([]byte(msg))
}
