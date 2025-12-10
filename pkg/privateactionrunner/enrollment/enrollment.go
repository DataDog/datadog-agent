// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/DataDog/jsonapi"
	"github.com/go-jose/go-jose/v4"
	"gopkg.in/yaml.v2"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

const (
	createRunnerPath  = "/api/unstable/on_prem_runners"
	versionHeaderName = "X-PAR-Version"
)

type MainConfigYaml struct {
	ApiKey              string           `yaml:"api_key"`
	Site                string           `yaml:"site"`
	PrivateActionRunner RunnerConfigYaml `yaml:"privateactionrunner"`
}

// RunnerConfigYaml represents the runner configuration structure
type RunnerConfigYaml struct {
	Urn              string   `yaml:"urn"`
	PrivateKey       string   `yaml:"private_key"`
	ActionsAllowlist []string `yaml:"actions_allowlist"`
	Enabled          bool     `yaml:"enabled"`
}

// CreateRunnerResponse represents the API response for creating a runner
type CreateRunnerResponse struct {
	ID       string `jsonapi:"primary,createRunnerResponse"`
	RunnerID string `json:"runner_id" jsonapi:"attribute"`
	OrgID    int64  `json:"org_id" jsonapi:"attribute"`
}

// generateKeys generates a new ECDSA key pair and returns private and public JWKs
func generateKeys() (*jose.JSONWebKey, *jose.JSONWebKey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	privateJwk, err := util.EcdsaToJWK(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert private key to JWK: %w", err)
	}

	publicJwk, err := util.EcdsaToJWK(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert public key to JWK: %w", err)
	}

	return privateJwk, publicJwk, nil
}

// ProvisionRunnerIdentityWithAPIKey enrolls a runner using API key and app key authentication
func ProvisionRunnerIdentityWithAPIKey(apiKey, appKey, site, runnerName, actionsAllowlist, connectionGroupID string) error {

	conf, err := RunEnrollmentWithAPIKey(site, apiKey, appKey, runnerName, actionsAllowlist, connectionGroupID)
	if err != nil {
		return err
	}

	log.Info("Enrollment complete, printing configuration...")

	confYaml, err := yaml.Marshal(conf)
	if err != nil {
		return err
	}

	fmt.Printf("copy the following values in your datadog.yaml:\n========== Runner Config ==========\n%s\n===================================\n", confYaml)
	return nil
}

// RunEnrollmentWithAPIKey enrolls a runner using API key and application key authentication
func RunEnrollmentWithAPIKey(site, apiKey, appKey, runnerName, actionsAllowlistStr, connectionGroupID string) (*MainConfigYaml, error) {
	actionsAllowlist := strings.Split(actionsAllowlistStr, ",")

	log.Info("Enrolling runner with API key authentication",
		log.String("runner_name", runnerName),
		log.Int("allowlist_count", len(actionsAllowlist)))

	log.Info("generating keys...")

	privateJwk, publicJwk, err := generateKeys()
	if err != nil {
		return nil, err
	}
	marshalledPrivateJwk, err := privateJwk.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	log.Info("converting public key to PEM format...")

	publicKeyPEM, err := util.JWKToPEM(publicJwk)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key to PEM: %w", err)
	}

	log.Info("building runner creation request...")

	createRunnerUrl := url.URL{
		Host:   "api." + site,
		Scheme: "https",
		Path:   createRunnerPath,
	}

	// Build JSON:API request body
	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "createRunnerRequest",
			"attributes": map[string]interface{}{
				"runner_name":              runnerName,
				"runner_modes":             []string{modes.ModePull.MetricTag()},
				"public_key_pem":           publicKeyPEM,
				"join_connection_group_id": connectionGroupID,
			},
		},
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	log.Info("sending runner creation request...")

	req, err := http.NewRequest("POST", createRunnerUrl.String(), strings.NewReader(string(requestBodyJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to build runner creation request: %w", err)
	}
	defer func() {
		if req.Body != nil {
			req.Body.Close()
		}
	}()

	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-APPLICATION-KEY", appKey)
	req.Header.Set(versionHeaderName, parversion.RunnerVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send runner creation request: %w", err)
	}
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("runner creation failed with HTTP status code %d and failed to read HTTP response with error %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runner creation failed with HTTP status code %d and response %s", resp.StatusCode, string(respBody))
	}

	createRunnerResponse := new(CreateRunnerResponse)
	err = jsonapi.Unmarshal(respBody, createRunnerResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal runner creation response: %w", err)
	}

	log.Info("Successfully created runner", log.String("runner_id", createRunnerResponse.RunnerID))

	return &MainConfigYaml{
		ApiKey: apiKey,
		Site:   site,
		PrivateActionRunner: RunnerConfigYaml{
			Urn:              fmt.Sprintf("urn:dd:apps:on-prem-runner:%s:%d:%s", siteToRegion(site), createRunnerResponse.OrgID, createRunnerResponse.RunnerID),
			PrivateKey:       base64.RawURLEncoding.EncodeToString(marshalledPrivateJwk),
			ActionsAllowlist: actionsAllowlist,
			Enabled:          true,
		},
	}, nil
}

func siteToRegion(site string) string {
	if strings.HasSuffix(site, ".datadoghq.com") {
		region := strings.TrimSuffix(site, ".datadoghq.com")
		return region
	} else if site == "datadoghq.eu" {
		return "eu1"
	}
	return "us1"
}
