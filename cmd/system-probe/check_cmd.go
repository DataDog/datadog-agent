package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

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
	if cfg.SystemProbeAddress == "" {
		return errors.New("no sysprobe_socket has been specified in system-probe.yaml")
	}

	httpClient := http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", cfg.SystemProbeAddress)
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

	// print json to stdout
	var out bytes.Buffer
	json.Indent(&out, body, "", "  ") //nolint:errcheck
	out.WriteTo(os.Stdout)            //nolint:errcheck

	return nil
}
