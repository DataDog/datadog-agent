// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Mock HTTP server for testing AnomalyDetector
func createMockRRCFServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reset":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))

		case "/add_point":
			// Parse request
			var req AddPointRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request"})
				return
			}

			// Calculate a simple score based on how far values are from 1.0
			// Values close to 1.0 (normal) get low scores
			// Values far from 1.0 (anomalous) get high scores
			avgDeviation := 0.0
			for _, v := range req.Values {
				avgDeviation += (1.0 - v) * (1.0 - v)
			}
			avgDeviation /= float64(len(req.Values))
			score := avgDeviation * 10.0 // Scale it up

			resp := AddPointResponse{CodispScore: score}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAnomalyDetector_NormalResults(t *testing.T) {
	server := createMockRRCFServer()
	defer server.Close()

	detector, err := NewAnomalyDetector(server.URL, 10, 50)
	AssertNoError(t, err, "NewAnomalyDetector")

	normalResults := CreateNormalResults(1)
	score, err := detector.ComputeScore(normalResults[0])
	AssertNoError(t, err, "ComputeScore with normal results")

	// Normal results (all 1.0) should have low score (based on mock server logic)
	if score > 1.0 {
		t.Errorf("Expected normal results to have low score, got %.3f", score)
	}

	// Score should be non-negative
	if score < 0 {
		t.Errorf("Score should be non-negative, got %.3f", score)
	}
}

func TestAnomalyDetector_AnomalousResults(t *testing.T) {
	server := createMockRRCFServer()
	defer server.Close()

	detector, err := NewAnomalyDetector(server.URL, 10, 50)
	AssertNoError(t, err, "NewAnomalyDetector")

	anomalousResults := CreateAnomalousResults(1)
	score, err := detector.ComputeScore(anomalousResults[0])
	AssertNoError(t, err, "ComputeScore with anomalous results")

	// Anomalous results should have higher score than normal
	normalResults := CreateNormalResults(1)
	normalScore, err := detector.ComputeScore(normalResults[0])
	AssertNoError(t, err, "ComputeScore with normal results")

	if score <= normalScore {
		t.Errorf("Anomalous score (%.3f) should be > normal score (%.3f)", score, normalScore)
	}
}

func TestAnomalyDetector_Reset(t *testing.T) {
	server := createMockRRCFServer()
	defer server.Close()

	detector, err := NewAnomalyDetector(server.URL, 10, 50)
	AssertNoError(t, err, "NewAnomalyDetector")

	// Test reset with new parameters
	err = detector.Reset(20, 100)
	AssertNoError(t, err, "Reset")

	// Verify detector still works after reset
	results := CreateNormalResults(1)
	score, err := detector.ComputeScore(results[0])
	AssertNoError(t, err, "ComputeScore after reset")

	if score < 0 {
		t.Errorf("Score should be non-negative, got %.3f", score)
	}
}

func TestAnomalyDetector_ServerError(t *testing.T) {
	// Create a server that always returns errors
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "server error"})
	}))
	defer errorServer.Close()

	_, err := NewAnomalyDetector(errorServer.URL, 10, 50)
	if err == nil {
		t.Error("Expected error when server returns error on reset")
	}
}

func TestAnomalyDetector_InvalidURL(t *testing.T) {
	_, err := NewAnomalyDetector("http://invalid-url-that-does-not-exist:9999", 10, 50)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestAnomalyDetector_MultipleResults(t *testing.T) {
	server := createMockRRCFServer()
	defer server.Close()

	detector, err := NewAnomalyDetector(server.URL, 10, 50)
	AssertNoError(t, err, "NewAnomalyDetector")

	// Pass multiple results, but only last one should be used
	results := CreateNormalResults(10)
	score, err := detector.ComputeScore(results[len(results)-1])
	AssertNoError(t, err, "ComputeScore with multiple results")

	// Should use only the last result (which is normal)
	if score > 1.0 {
		t.Errorf("Expected low score for normal data, got %.3f", score)
	}
}

func TestAnomalyDetector_MixedResults(t *testing.T) {
	server := createMockRRCFServer()
	defer server.Close()

	detector, err := NewAnomalyDetector(server.URL, 10, 50)
	AssertNoError(t, err, "NewAnomalyDetector")

	// Create mixed results (normal then anomalous)
	mixedResults := CreateMixedResults()
	score, err := detector.ComputeScore(mixedResults[len(mixedResults)-1])
	AssertNoError(t, err, "ComputeScore with mixed results")

	// Last result is anomalous, so score should be high
	if score <= 1.0 {
		t.Errorf("Expected high score for anomalous data, got %.3f", score)
	}
}

func TestAnomalyDetector_BadResponseFormat(t *testing.T) {
	// Create server that returns invalid JSON
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reset" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		} else if r.URL.Path == "/add_point" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`invalid json`))
		}
	}))
	defer badServer.Close()

	detector, err := NewAnomalyDetector(badServer.URL, 10, 50)
	AssertNoError(t, err, "NewAnomalyDetector")

	results := CreateNormalResults(1)
	_, err = detector.ComputeScore(results[0])
	if err == nil {
		t.Error("Expected error for invalid JSON response")
	}
}
