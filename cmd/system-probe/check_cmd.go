package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

var (
	checkEndpoints = map[string]string{
		"network_maps":  "http://unix/debug/net_maps",
		"stats":         "http://unix/debug/stats",
		"network_state": "http://unix/debug/net_state",
		"connections":   "http://unix/connections",
	}
)

func querySocketEndpoint(cfg *config.AgentConfig, check string, client string) error {
	if cfg.SystemProbeSocketPath == "" {
		return errors.New("No sysprobe_socket has been specified in system-probe.yaml")
	}

	httpClient := http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", cfg.SystemProbeSocketPath)
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}

	endpoint, ok := checkEndpoints[check]
	if !ok {
		return fmt.Errorf("unknown check requested: %s", check)
	}

	if client != "" {
		endpoint = fmt.Sprintf("%s?client_id=%s", endpoint, client)
	}

	resp, err := httpClient.Get(endpoint)
	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("check request failed: check: %s", check)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err = printResult(body, check); err != nil {
		return err
	}
	return nil
}

func printResult(r []byte, check string) error {
	var content interface{}

	switch check {
	case "network_maps", "connections":
		conn := &ebpf.Connections{}
		if err := conn.UnmarshalJSON(r); err != nil {
			return err
		}
		content = conn
	case "stats", "network_state":
		var output map[string]interface{}
		if err := json.Unmarshal(r, &output); err != nil {
			return err
		}
		content = output
	}

	b, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(b))
	return nil
}
