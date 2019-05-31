// +build linux

package net

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	statusURL      = "http://unix/status"
	connectionsURL = "http://unix/connections"
)

var (
	globalUtil            *RemoteSysProbeUtil
	globalSocketPath      string
	hasLoggedErrForStatus map[retry.Status]struct{}
)

func init() {
	hasLoggedErrForStatus = make(map[retry.Status]struct{})
}

// RemoteSysProbeUtil wraps interactions with a remote system probe service
type RemoteSysProbeUtil struct {
	// Retrier used to setup system probe
	initRetry retry.Retrier

	socketPath string
	httpClient http.Client

	// buf is a shared buffer used when reading JSON off the unix-socket. mu is used to synchronize
	// access to buf
	buf *bytes.Buffer
	mu  sync.Mutex
}

// SetSystemProbeSocketPath provides a unix socket path location to be used by the remote system probe.
// This needs to be called before GetRemoteSystemProbeUtil.
func SetSystemProbeSocketPath(socketPath string) {
	globalSocketPath = socketPath
}

// GetRemoteSystemProbeUtil returns a ready to use RemoteSysProbeUtil. It is backed by a shared singleton.
func GetRemoteSystemProbeUtil() (*RemoteSysProbeUtil, error) {
	if globalSocketPath == "" {
		return nil, fmt.Errorf("remote tracer has no socket path defined")
	}

	if globalUtil == nil {
		globalUtil = newSystemProbe()
		globalUtil.initRetry.SetupRetrier(&retry.Config{
			Name:          "system-probe-util",
			AttemptMethod: globalUtil.init,
			Strategy:      retry.RetryCount,
			// 10 tries w/ 30s delays = 5m of trying before permafail
			RetryCount: 10,
			RetryDelay: 30 * time.Second,
		})
	}

	if err := globalUtil.initRetry.TriggerRetry(); err != nil {
		log.Debugf("system probe init error: %s", err)
		return nil, err
	}

	return globalUtil, nil
}

// GetConnections returns a set of active network connections, retrieved from the system probe service
func (r *RemoteSysProbeUtil) GetConnections(clientID string) ([]ebpf.ConnectionStats, error) {
	resp, err := r.httpClient.Get(fmt.Sprintf("%s?client_id=%s", connectionsURL, clientID))
	if err != nil {
		return nil, err
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: socket %s, url: %s, status code: %d", r.socketPath, connectionsURL, resp.StatusCode)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.Reset()
	r.buf.ReadFrom(resp.Body)

	conn := &ebpf.Connections{}
	if err := conn.UnmarshalJSON(r.buf.Bytes()); err != nil {
		return nil, err
	}

	// shrink buffer if necessary. Otherwise, a buf sized for the largest possible
	// entry will stick around
	if r.buf.Len() < r.buf.Cap()/2 {
		r.buf = bytes.NewBuffer(make([]byte, 0, r.buf.Cap()/2))
	}

	return conn.Conns, nil
}

// ShouldLogTracerUtilError will return whether or not errors sourced from the RemoteSysProbeUtil _should_ be logged, for less noisy logging.
// We only want to log errors if the tracer has been initialized, or it's the first error for a particular tracer status
// (e.g. retrying, permafail)
func ShouldLogTracerUtilError() bool {
	status := globalUtil.initRetry.RetryStatus()

	_, logged := hasLoggedErrForStatus[status]
	hasLoggedErrForStatus[status] = struct{}{}

	return status == retry.OK || !logged
}

func newSystemProbe() *RemoteSysProbeUtil {
	return &RemoteSysProbeUtil{
		socketPath: globalSocketPath,
		buf:        &bytes.Buffer{},
		httpClient: http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:    2,
				IdleConnTimeout: 30 * time.Second,
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", globalSocketPath)
				},
				TLSHandshakeTimeout:   1 * time.Second,
				ResponseHeaderTimeout: 5 * time.Second,
				ExpectContinueTimeout: 50 * time.Millisecond,
			},
		},
	}
}

func (r *RemoteSysProbeUtil) init() error {
	if resp, err := r.httpClient.Get(statusURL); err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote tracer status check failed: socket %s, url: %s, status code: %d", r.socketPath, statusURL, resp.StatusCode)
	}
	return nil
}
