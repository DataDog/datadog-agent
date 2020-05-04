// +build linux windows

package net

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Conn is a wrapper over some net.Listener
type Conn interface {
	// GetListener returns the underlying net.Listener
	GetListener() net.Listener

	// Stop and clean up resources for the underlying connection
	Stop()
}

const (
	contentTypeProtobuf = "application/protobuf"
)

var (
	globalUtil       *RemoteSysProbeUtil
	globalUtilOnce   sync.Once
	globalSocketPath string
)

// RemoteSysProbeUtil wraps interactions with a remote system probe service
type RemoteSysProbeUtil struct {
	// Retrier used to setup system probe
	initRetry retry.Retrier

	path       string
	httpClient http.Client
}

// SetSystemProbePath sets where the System probe is listening for connections
// This needs to be called before GetRemoteSystemProbeUtil.
func SetSystemProbePath(path string) {
	globalSocketPath = path
}

// GetRemoteSystemProbeUtil returns a ready to use RemoteSysProbeUtil. It is backed by a shared singleton.
func GetRemoteSystemProbeUtil() (*RemoteSysProbeUtil, error) {
	err := CheckPath()
	if err != nil {
		return nil, fmt.Errorf("error setting up remote system probe util, %v", err)
	}

	globalUtilOnce.Do(func() {
		globalUtil = newSystemProbe()
		globalUtil.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:          "system-probe-util",
			AttemptMethod: globalUtil.init,
			Strategy:      retry.RetryCount,
			// 10 tries w/ 30s delays = 5m of trying before permafail
			RetryCount: 10,
			RetryDelay: 30 * time.Second,
		})
	})

	if err := globalUtil.initRetry.TriggerRetry(); err != nil {
		log.Debugf("system probe init error: %s", err)
		return nil, err
	}

	return globalUtil, nil
}

// GetConnections returns a set of active network connections, retrieved from the system probe service
func (r *RemoteSysProbeUtil) GetConnections(clientID string) (*model.Connections, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?client_id=%s", connectionsURL, clientID), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeProtobuf)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: Probe Path %s, url: %s, status code: %d", r.path, connectionsURL, resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	conns, err := encoding.GetUnmarshaler(contentType).Unmarshal(body)
	if err != nil {
		return nil, err
	}

	return conns, nil
}

// GetStats returns the expvar stats of the system probe
func (r *RemoteSysProbeUtil) GetStats() (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", statsURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: Path %s, url: %s, status code: %d", r.path, statsURL, resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	err = json.Unmarshal(body, &stats)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func newSystemProbe() *RemoteSysProbeUtil {
	return &RemoteSysProbeUtil{
		path: globalSocketPath,
		httpClient: http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:    2,
				IdleConnTimeout: 30 * time.Second,
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial(netType, globalSocketPath)
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
		return fmt.Errorf("remote tracer status check failed: socket %s, url: %s, status code: %d", r.path, statusURL, resp.StatusCode)
	}
	return nil
}
