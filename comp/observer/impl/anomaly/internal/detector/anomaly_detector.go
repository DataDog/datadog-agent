// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnomalyDetector is a client for the RRCF-based anomaly detection service
type AnomalyDetector struct {
	baseURL    string
	httpClient *http.Client
	numTrees   int
	windowSize int
}

// ResetConfig holds configuration for the detector
type ResetConfig struct {
	NumTrees   int `json:"num_trees"`
	WindowSize int `json:"window_size"`
}

// AddPointRequest represents the request body for adding a data point
type AddPointRequest struct {
	Values []float64 `json:"values"`
}

// AddPointResponse represents the response from the add_point endpoint
type AddPointResponse struct {
	CodispScore float64 `json:"codisp_score"`
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Error string `json:"error"`
}

// NewAnomalyDetector creates a new AnomalyDetector and initializes it by calling the reset endpoint
func NewAnomalyDetector(baseURL string, numTrees, windowSize int) (*AnomalyDetector, error) {
	detector := &AnomalyDetector{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		numTrees:   numTrees,
		windowSize: windowSize,
	}

	// Call reset endpoint to initialize the detector
	if err := detector.reset(); err != nil {
		return nil, fmt.Errorf("failed to initialize detector: %w", err)
	}

	return detector, nil
}

// reset calls the /reset endpoint to initialize or reconfigure the detector
func (ad *AnomalyDetector) reset() error {
	config := ResetConfig{
		NumTrees:   ad.numTrees,
		WindowSize: ad.windowSize,
	}

	jsonData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal reset config: %w", err)
	}

	resp, err := ad.httpClient.Post(
		ad.baseURL+"/reset",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to call reset endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return fmt.Errorf("reset failed: %s", errResp.Error)
		}
		return fmt.Errorf("reset failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ComputeScore sends a data point to the detector and returns its CoDisp score
func (ad *AnomalyDetector) ComputeScore(result TelemetryResult) (float64, error) {
	// Convert result to an array
	values := result.ToArray()

	request := AddPointRequest{
		Values: values,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := ad.httpClient.Post(
		ad.baseURL+"/add_point",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to call add_point endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return 0, fmt.Errorf("add_point failed: %s", errResp.Error)
		}
		return 0, fmt.Errorf("add_point failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response AddPointResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response.CodispScore, nil
}

// Name returns the name of this detector
func (ad *AnomalyDetector) Name() string {
	return "RRCF"
}

// HigherIsAnomalous returns true since higher RRCF scores indicate anomalies
func (ad *AnomalyDetector) HigherIsAnomalous() bool {
	return true
}

// Reset allows reconfiguring the detector at runtime
func (ad *AnomalyDetector) Reset(numTrees, windowSize int) error {
	ad.numTrees = numTrees
	ad.windowSize = windowSize
	return ad.reset()
}
