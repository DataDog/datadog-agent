// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package localapiclientimpl provides the installer local API client component.
package localapiclientimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
	localapiclient "github.com/DataDog/datadog-agent/comp/updater/localapiclient/def"
)

const (
	serverAddress         = "daemon"
	statusRequestTimeout  = 2 * time.Second
	maxStatusResponseSize = 1 << 20
)

type experimentTaskParams struct {
	Version          string            `json:"version"`
	InstallArgs      []string          `json:"install_args"`
	EncryptedSecrets []encryptedSecret `json:"encrypted_secrets"`
}

type encryptedSecret struct {
	Key            string `json:"key"`
	EncryptedValue string `json:"encrypted_value"`
}

type startConfigExperimentRequest struct {
	Operations       string            `json:"operations"`
	EncryptedSecrets map[string]string `json:"encrypted_secrets"`
}

// Requires defines the dependencies for the localapiclient component.
type Requires struct{}

// Provides defines the outputs of the localapiclient component.
type Provides struct {
	Comp         localapiclient.Component
	StatusClient localapiclient.StatusClient
}

type localAPIClient struct {
	client *http.Client
	addr   string
}

// NewComponent creates a new localapiclient component.
func NewComponent(_ Requires) Provides {
	client := newClient(newHTTPClient(), serverAddress)
	return Provides{
		Comp:         client,
		StatusClient: client,
	}
}

// NewClient creates a local API client with the supplied HTTP client and address.
// The default component uses the installer socket or named pipe instead.
func NewClient(client *http.Client, addr string) localapiclient.Component {
	return newClient(client, addr)
}

func newClient(client *http.Client, addr string) *localAPIClient {
	return &localAPIClient{client: client, addr: addr}
}

// Status returns the status of the installer daemon.
func (c *localAPIClient) Status() (localapi.StatusResponse, error) {
	var response localapi.StatusResponse

	ctx, cancel := context.WithTimeout(context.Background(), statusRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(localapi.StatusEndpoint), nil)
	if err != nil {
		return response, err
	}
	setJSONHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxStatusResponseSize+1))
	if err != nil {
		return response, fmt.Errorf("reading installer status response: %w", err)
	}
	if len(body) > maxStatusResponseSize {
		return response, fmt.Errorf("installer status response exceeds %d bytes", maxStatusResponseSize)
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return response, fmt.Errorf("decoding installer status response: %w", err)
	}
	if response.Error != nil {
		return response, fmt.Errorf("error getting status: %s", response.Error.Message)
	}
	return response, nil
}

// SetCatalog sets the catalog for the daemon.
func (c *localAPIClient) SetCatalog(catalog string) error {
	return c.do(http.MethodPost, "/catalog", bytes.NewBufferString(catalog), "error setting catalog")
}

// SetConfigCatalog sets the config catalog for the daemon.
func (c *localAPIClient) SetConfigCatalog(configs string) error {
	return c.do(http.MethodPost, "/config_catalog", bytes.NewBufferString(configs), "error setting config catalog")
}

// Install installs a package with a specific version.
func (c *localAPIClient) Install(pkg, version string) error {
	return c.doJSON(fmt.Sprintf("/%s/install", pkg), experimentTaskParams{Version: version}, "error installing")
}

// Remove removes a package.
func (c *localAPIClient) Remove(pkg string) error {
	return c.do(http.MethodPost, fmt.Sprintf("/%s/remove", pkg), nil, "error removing")
}

// StartExperiment starts an experiment for a package.
func (c *localAPIClient) StartExperiment(pkg, version string) error {
	return c.doJSON(fmt.Sprintf("/%s/experiment/start", pkg), experimentTaskParams{Version: version}, "error starting experiment")
}

// StopExperiment stops an experiment for a package.
func (c *localAPIClient) StopExperiment(pkg string) error {
	return c.do(http.MethodPost, fmt.Sprintf("/%s/experiment/stop", pkg), nil, "error stopping experiment")
}

// PromoteExperiment promotes an experiment for a package.
func (c *localAPIClient) PromoteExperiment(pkg string) error {
	return c.do(http.MethodPost, fmt.Sprintf("/%s/experiment/promote", pkg), nil, "error promoting experiment")
}

// StartConfigExperiment starts a configuration experiment for a package.
func (c *localAPIClient) StartConfigExperiment(pkg, operations string, encryptedSecrets map[string]string) error {
	request := startConfigExperimentRequest{
		Operations:       operations,
		EncryptedSecrets: encryptedSecrets,
	}
	return c.doJSON(fmt.Sprintf("/%s/config_experiment/start", pkg), request, "error starting config experiment")
}

// StopConfigExperiment stops a configuration experiment for a package.
func (c *localAPIClient) StopConfigExperiment(pkg string) error {
	return c.do(http.MethodPost, fmt.Sprintf("/%s/config_experiment/stop", pkg), nil, "error stopping config experiment")
}

// PromoteConfigExperiment promotes a configuration experiment for a package.
func (c *localAPIClient) PromoteConfigExperiment(pkg string) error {
	return c.do(http.MethodPost, fmt.Sprintf("/%s/config_experiment/promote", pkg), nil, "error promoting config experiment")
}

func (c *localAPIClient) doJSON(path string, value any, errorPrefix string) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.do(http.MethodPost, path, bytes.NewReader(body), errorPrefix)
}

func (c *localAPIClient) do(method, path string, body io.Reader, errorPrefix string) error {
	req, err := http.NewRequest(method, c.url(path), body)
	if err != nil {
		return err
	}
	setJSONHeaders(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var response localapi.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("%s: %s", errorPrefix, response.Error.Message)
	}
	return nil
}

func (c *localAPIClient) url(path string) string {
	return fmt.Sprintf("http://%s%s", c.addr, path)
}

func setJSONHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
