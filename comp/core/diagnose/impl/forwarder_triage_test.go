// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package diagnoseimpl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func TestTopEndpointsDiagnosis_MetadataShapeAndOrdering(t *testing.T) {
	d := forwarderDelta{
		RequeuedByEndpoint: map[string]int{
			"series_v2":   10,
			"metadata_v1": 2,
			"intake":      7,
		},
		RetriedByEndpoint: map[string]int{
			"series_v2": 3,
		},
		DroppedByEndpoint: map[string]int{},
	}

	diag := topEndpointsDiagnosis(d, diagnose.DiagnosisWarning)
	if diag.Status != diagnose.DiagnosisWarning {
		t.Fatalf("expected WARNING, got %v", diag.Status)
	}

	raw, ok := diag.Metadata["top_requeued_by_endpoint"]
	if !ok || raw == "" {
		t.Fatalf("expected top_requeued_by_endpoint metadata to be present")
	}

	type endpointCount struct {
		Endpoint string `json:"endpoint"`
		Count    int    `json:"count"`
	}
	var got []endpointCount
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("failed to unmarshal top_requeued_by_endpoint: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %#v", len(got), got)
	}

	if got[0].Endpoint != "series_v2" || got[0].Count != 10 {
		t.Fatalf("unexpected first entry: %#v", got[0])
	}
	if got[1].Endpoint != "intake" || got[1].Count != 7 {
		t.Fatalf("unexpected second entry: %#v", got[1])
	}
	if got[2].Endpoint != "metadata_v1" || got[2].Count != 2 {
		t.Fatalf("unexpected third entry: %#v", got[2])
	}
}

func TestForwarderTriageSuite_Verbose_UnhealthyAndDetailsWarning(t *testing.T) {
	origSleep := sleepFn
	sleepFn = func(_ time.Duration) {}
	defer func() { sleepFn = origSleep }()

	var calls int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt64(&calls, 1)

		payload := map[string]any{
			"forwarder": map[string]any{
				"APIKeyStatus": map[string]string{
					"API key ending with 742ce": "Unable to reach the API Key validation endpoint",
				},
				"Transactions": map[string]any{
					"Success":               0,
					"Errors":                int(10 * n),
					"Dropped":               0,
					"Requeued":              int(50 * n),
					"Retried":               int(5 * n),
					"RetryQueueSize":        0,
					"HighPriorityQueueFull": 0,
					"ConnectionEvents": map[string]any{
						"ConnectSuccess": 0,
						"DNSSuccess":     int(1 * n),
					},
					"ErrorsByType": map[string]any{
						"ConnectionErrors":   int(10 * n),
						"DNSErrors":          int(1 * n),
						"SentRequestErrors":  0,
						"TLSErrors":          0,
						"WroteRequestErrors": 0,
					},
					"HTTPErrorsByCode": map[string]any{},
					"RequeuedByEndpoint": map[string]any{
						"series_v2": int(10 * n),
					},
					"RetriedByEndpoint": map[string]any{
						"series_v2": int(3 * n),
					},
					"DroppedByEndpoint": map[string]any{},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	prev := os.Getenv("DD_DIAGNOSE_EXPVAR_URL")
	_ = os.Setenv("DD_DIAGNOSE_EXPVAR_URL", srv.URL)
	defer func() {
		if prev == "" {
			_ = os.Unsetenv("DD_DIAGNOSE_EXPVAR_URL")
		} else {
			_ = os.Setenv("DD_DIAGNOSE_EXPVAR_URL", prev)
		}
	}()

	diags := ForwarderTriageSuite(diagnose.Config{Verbose: true})
	if len(diags) != 3 {
		t.Fatalf("expected 3 diagnoses, got %d: %#v", len(diags), diags)
	}

	if diags[0].Name != "Forwarder API key status" || diags[0].Status != diagnose.DiagnosisWarning {
		t.Fatalf("unexpected api key diagnosis: %#v", diags[0])
	}

	if diags[1].Name != "Forwarder health" || diags[1].Status != diagnose.DiagnosisFail {
		t.Fatalf("unexpected health diagnosis: %#v", diags[1])
	}

	if diags[2].Name != "Forwarder top impacted endpoints" || diags[2].Status != diagnose.DiagnosisWarning {
		t.Fatalf("unexpected details diagnosis: %#v", diags[2])
	}

	rawErrs := diags[1].Metadata["errors_by_type_delta"]
	var errs map[string]int
	if err := json.Unmarshal([]byte(rawErrs), &errs); err != nil {
		t.Fatalf("failed to unmarshal errors_by_type_delta: %v", err)
	}
	if errs["ConnectionErrors"] != 10 {
		t.Fatalf("expected ConnectionErrors delta 10, got %d (%v)", errs["ConnectionErrors"], errs)
	}

	rawTop := diags[2].Metadata["top_requeued_by_endpoint"]
	type endpointCount struct {
		Endpoint string `json:"endpoint"`
		Count    int    `json:"count"`
	}
	var tops []endpointCount
	if err := json.Unmarshal([]byte(rawTop), &tops); err != nil {
		t.Fatalf("failed to unmarshal top_requeued_by_endpoint: %v", err)
	}
	if len(tops) != 1 || tops[0].Endpoint != "series_v2" || tops[0].Count != 10 {
		t.Fatalf("unexpected tops: %#v", tops)
	}
}

func TestFetchForwarder_ParsesNestedForwarderKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := map[string]any{
			"forwarder": map[string]any{
				"APIKeyStatus": map[string]string{"k": "API Key valid"},
				"Transactions": map[string]any{
					"Success": 1,
					"Errors":  0,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	snap, err := fetchForwarder(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Transactions.Success != 1 {
		t.Fatalf("expected Success=1, got %d", snap.Transactions.Success)
	}
}

func TestFetchForwarder_FallbackRootObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := map[string]any{
			"APIKeyStatus": map[string]string{"k": "API Key valid"},
			"Transactions": map[string]any{
				"Success": 2,
				"Errors":  3,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	snap, err := fetchForwarder(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Transactions.Success != 2 || snap.Transactions.Errors != 3 {
		t.Fatalf("unexpected transactions: success=%d errors=%d", snap.Transactions.Success, snap.Transactions.Errors)
	}
}
