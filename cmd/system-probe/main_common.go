package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

func querySocketEndpoint(cfg *config.AgentConfig, endpoint string) error {
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

	url := endpoint
	if !strings.HasPrefix(url, "http://") {
		url = fmt.Sprintf("http://%s", url)
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("conn request failed: url: %s, status code: %d", url, resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err = printResult(body, url); err != nil {
		return err
	}
	return nil
}

func printResult(r []byte, endpoint string) error {
	var content interface{}

	switch endpoint {
	case "http://unix/debug/net_maps":
		fallthrough
	case "http://unix/connections":
		conn := &ebpf.Connections{}
		if err := conn.UnmarshalJSON(r); err != nil {
			return err
		}
		content = conn
	case "http://unix/debug/stats":
		fallthrough
	case "http://unix/debug/net_state":
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
