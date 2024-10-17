// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package net

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"google.golang.org/protobuf/proto"

	discoverymodel "github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	netEncoding "github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	nppayload "github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	procEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding"
	reqEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding/request"
	languagepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/languagedetection"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
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
	contentTypeJSON     = "application/json"
)

var (
	globalUtil     *RemoteSysProbeUtil
	globalUtilOnce sync.Once
)

var _ SysProbeUtil = &RemoteSysProbeUtil{}

// RemoteSysProbeUtil wraps interactions with a remote system probe service
type RemoteSysProbeUtil struct {
	// Retrier used to setup system probe
	initRetry retry.Retrier

	path             string
	httpClient       http.Client
	pprofClient      http.Client
	tracerouteClient http.Client
}

// ensure that GetRemoteSystemProbeUtil implements SysProbeUtilGetter
var _ SysProbeUtilGetter = GetRemoteSystemProbeUtil

// GetRemoteSystemProbeUtil returns a ready to use RemoteSysProbeUtil. It is backed by a shared singleton.
func GetRemoteSystemProbeUtil(path string) (SysProbeUtil, error) {
	err := CheckPath(path)
	if err != nil {
		return nil, fmt.Errorf("error setting up remote system probe util, %v", err)
	}

	globalUtilOnce.Do(func() {
		globalUtil = newSystemProbe(path)
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

// GetProcStats returns a set of process stats by querying system-probe
func (r *RemoteSysProbeUtil) GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error) {
	procReq := &pbgo.ProcessStatRequest{
		Pids: pids,
	}

	reqBody, err := reqEncoding.GetMarshaler(reqEncoding.ContentTypeProtobuf).Marshal(procReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", procStatsURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeProtobuf)
	req.Header.Set("Content-Type", procEncoding.ContentTypeProtobuf)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proc_stats request failed: Probe Path %s, url: %s, status code: %d", r.path, procStatsURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	results, err := procEncoding.GetUnmarshaler(contentType).Unmarshal(body)
	if err != nil {
		return nil, err
	}

	return results, nil
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
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: Probe Path %s, url: %s, status code: %d", r.path, connectionsURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	conns, err := netEncoding.GetUnmarshaler(contentType).Unmarshal(body)
	if err != nil {
		return nil, err
	}

	return conns, nil
}

// GetNetworkID fetches the network_id (vpc_id) from system-probe
func (r *RemoteSysProbeUtil) GetNetworkID() (string, error) {
	req, err := http.NewRequest("GET", networkIDURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/plain")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("network_id request failed: url: %s, status code: %d", networkIDURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// GetPing returns the results of a ping to a host
func (r *RemoteSysProbeUtil) GetPing(clientID string, host string, count int, interval time.Duration, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s?client_id=%s&count=%d&interval=%d&timeout=%d", pingURL, host, clientID, count, interval, timeout), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeJSON)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("ping request failed: Probe Path %s, url: %s, status code: %d", r.path, pingURL, resp.StatusCode)
		}
		return nil, fmt.Errorf("ping request failed: Probe Path %s, url: %s, status code: %d, error: %s", r.path, pingURL, resp.StatusCode, string(body))
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ping request failed: Probe Path %s, url: %s, status code: %d", r.path, pingURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// GetTraceroute returns the results of a traceroute to a host
func (r *RemoteSysProbeUtil) GetTraceroute(clientID string, host string, port uint16, protocol nppayload.Protocol, maxTTL uint8, timeout time.Duration) ([]byte, error) {
	httpTimeout := timeout*time.Duration(maxTTL) + 10*time.Second // allow extra time for the system probe communication overhead, calculate full timeout for TCP traceroute
	log.Tracef("Network Path traceroute HTTP request timeout: %s", httpTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/%s?client_id=%s&port=%d&max_ttl=%d&timeout=%d&protocol=%s", tracerouteURL, host, clientID, port, maxTTL, timeout, protocol), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", contentTypeJSON)
	resp, err := r.tracerouteClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("traceroute request failed: Probe Path %s, url: %s, status code: %d", r.path, tracerouteURL, resp.StatusCode)
		}
		return nil, fmt.Errorf("traceroute request failed: Probe Path %s, url: %s, status code: %d, error: %s", r.path, tracerouteURL, resp.StatusCode, string(body))
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traceroute request failed: Probe Path %s, url: %s, status code: %d", r.path, tracerouteURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
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
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: Path %s, url: %s, status code: %d", r.path, statsURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
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

// Register registers the client to system probe
func (r *RemoteSysProbeUtil) Register(clientID string) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?client_id=%s", registerURL, clientID), nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("conn request failed: Path %s, url: %s, status code: %d", r.path, statsURL, resp.StatusCode)
	}

	return nil
}

//nolint:revive // TODO(PROC) Fix revive linter
func (r *RemoteSysProbeUtil) DetectLanguage(pids []int32) ([]languagemodels.Language, error) {
	procs := make([]*languagepb.Process, len(pids))
	for i, pid := range pids {
		procs[i] = &languagepb.Process{Pid: pid}
	}
	reqBytes, err := proto.Marshal(&languagepb.DetectLanguageRequest{Processes: procs})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, languageDetectionURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}

	res, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var resProto languagepb.DetectLanguageResponse
	err = proto.Unmarshal(resBody, &resProto)
	if err != nil {
		return nil, err
	}

	langs := make([]languagemodels.Language, len(pids))
	for i, lang := range resProto.Languages {
		langs[i] = languagemodels.Language{
			Name:    languagemodels.LanguageName(lang.Name),
			Version: lang.Version,
		}
	}
	return langs, nil
}

// GetPprof queries the pprof endpoint for system-probe
func (r *RemoteSysProbeUtil) GetPprof(path string) ([]byte, error) {
	var buf bytes.Buffer
	req, err := http.NewRequest(http.MethodGet, pprofURL+path, &buf)
	if err != nil {
		return nil, err
	}

	res, err := r.pprofClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

// GetDiscoveryServices returns service information from system-probe.
func (r *RemoteSysProbeUtil) GetDiscoveryServices() (*discoverymodel.ServicesResponse, error) {
	req, err := http.NewRequest(http.MethodGet, discoveryServicesURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non-success status code: path %s, url: %s, status_code: %d", r.path, discoveryServicesURL, resp.StatusCode)
	}

	res := &discoverymodel.ServicesResponse{}
	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetTelemetry queries the telemetry endpoint from system-probe.
func (r *RemoteSysProbeUtil) GetTelemetry() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, telemetryURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`GetTelemetry got non-success status code: path %s, url: %s, status_code: %d, response: "%s"`, r.path, req.URL, resp.StatusCode, data)
	}

	return data, nil
}

// GetConnTrackCached queries conntrack/cached, which uses our conntracker implementation (typically ebpf)
// to return the list of NAT'd connections
func (r *RemoteSysProbeUtil) GetConnTrackCached() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, conntrackCachedURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`GetConnTrackCached got non-success status code: path %s, url: %s, status_code: %d, response: "%s"`, r.path, req.URL, resp.StatusCode, data)
	}

	return data, nil
}

// GetConnTrackHost queries conntrack/host, which uses netlink to return the list of NAT'd connections
func (r *RemoteSysProbeUtil) GetConnTrackHost() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, conntrackHostURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`GetConnTrackHost got non-success status code: path %s, url: %s, status_code: %d, response: "%s"`, r.path, req.URL, resp.StatusCode, data)
	}

	return data, nil
}

func (r *RemoteSysProbeUtil) init() error {
	resp, err := r.httpClient.Get(statsURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote tracer status check failed: socket %s, url: %s, status code: %d", r.path, statsURL, resp.StatusCode)
	}
	return nil
}
