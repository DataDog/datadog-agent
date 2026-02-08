// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package diagnoseimpl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

// sleepFn is overridden in unit tests to avoid real waiting.
var sleepFn = time.Sleep

type forwarderSnapshot struct {
	APIKeyStatus map[string]string `json:"APIKeyStatus"`
	Transactions struct {
		Success int `json:"Success"`
		Errors  int `json:"Errors"`

		Requeued int `json:"Requeued"`
		Retried  int `json:"Retried"`
		Dropped  int `json:"Dropped"`

		RetryQueueSize        int `json:"RetryQueueSize"`
		HighPriorityQueueFull int `json:"HighPriorityQueueFull"`

		ConnectionEvents struct {
			ConnectSuccess int `json:"ConnectSuccess"`
			DNSSuccess     int `json:"DNSSuccess"`
		} `json:"ConnectionEvents"`

		ErrorsByType         map[string]int `json:"ErrorsByType"`
		HTTPErrorsByCode     map[string]int `json:"HTTPErrorsByCode"`
		InputCountByEndpoint map[string]int `json:"InputCountByEndpoint"`
		RequeuedByEndpoint   map[string]int `json:"RequeuedByEndpoint"`
		RetriedByEndpoint    map[string]int `json:"RetriedByEndpoint"`
		DroppedByEndpoint    map[string]int `json:"DroppedByEndpoint"`
	} `json:"Transactions"`
}

func ForwarderTriageSuite(cfg diagnose.Config) []diagnose.Diagnosis {
	url := expvarURL()

	snap1, err := fetchForwarder(url)
	if err != nil {
		return []diagnose.Diagnosis{{
			Status:      diagnose.DiagnosisFail,
			Name:        "Forwarder expvar reachable",
			Category:    "forwarder",
			Description: "Fetch /debug/vars and parse forwarder expvar",
			Diagnosis:   "Unable to fetch forwarder expvar from " + url,
			RawError:    err.Error(),
			Remediation: "Check expvar_port / DD_EXPVAR_PORT, local firewall, and that the Agent is running with expvar enabled.",
		}}
	}

	window := 5 * time.Second
	if cfg.Verbose {
		window = 10 * time.Second
	}
	sleepFn(window)

	snap2, err := fetchForwarder(url)
	if err != nil {
		return []diagnose.Diagnosis{{
			Status:      diagnose.DiagnosisFail,
			Name:        "Forwarder expvar stable",
			Category:    "forwarder",
			Description: "Fetch /debug/vars twice and compute deltas",
			Diagnosis:   "First fetch succeeded but second fetch failed from " + url,
			RawError:    err.Error(),
			Remediation: "If this is intermittent, suspect local resource pressure or network rules affecting the expvar port.",
		}}
	}

	d := diffForwarder(snap1, snap2)

	diags := make([]diagnose.Diagnosis, 0, 4)

	apiDiag := apiKeyStatusDiagnosis(snap2)
	diags = append(diags, apiDiag)

	healthDiag := forwarderHealthDiagnosis(d, window)
	diags = append(diags, healthDiag)

	if cfg.Verbose {
		// Never emit PASS here if the suite is indicating trouble.
		detailsStatus := diagnose.DiagnosisSuccess
		if apiDiag.Status != diagnose.DiagnosisSuccess || healthDiag.Status != diagnose.DiagnosisSuccess {
			detailsStatus = diagnose.DiagnosisWarning
		}
		diags = append(diags, topEndpointsDiagnosis(d, detailsStatus))
	}

	return diags
}

func expvarURL() string {
	if u := os.Getenv("DD_DIAGNOSE_EXPVAR_URL"); u != "" {
		return u
	}
	port := os.Getenv("DD_EXPVAR_PORT")
	if port == "" {
		port = "5000"
	}
	return fmt.Sprintf("http://127.0.0.1:%s/debug/vars", port)
}

func fetchForwarder(url string) (forwarderSnapshot, error) {
	client := &http.Client{Timeout: 3 * time.Second}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return forwarderSnapshot{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return forwarderSnapshot{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return forwarderSnapshot{}, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	var root map[string]json.RawMessage
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&root); err != nil {
		return forwarderSnapshot{}, err
	}

	if raw, ok := root["forwarder"]; ok {
		var snap forwarderSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			return forwarderSnapshot{}, err
		}
		return snap, nil
	}

	rawAll, err := json.Marshal(root)
	if err != nil {
		return forwarderSnapshot{}, err
	}

	var snap forwarderSnapshot
	if err := json.Unmarshal(rawAll, &snap); err != nil {
		return forwarderSnapshot{}, fmt.Errorf("no 'forwarder' key and root didn't match forwarder schema: %w", err)
	}
	return snap, nil
}

type forwarderDelta struct {
	Window time.Duration

	Success int
	Errors  int

	Dropped  int
	Retried  int
	Requeued int

	RetryQueueSizeDelta   int
	HighPriorityQueueFull int

	ConnectSuccess int
	DNSSuccess     int

	ErrorsByType     map[string]int
	HTTPErrorsByCode map[string]int

	RequeuedByEndpoint map[string]int
	RetriedByEndpoint  map[string]int
	DroppedByEndpoint  map[string]int
}

func diffForwarder(a, b forwarderSnapshot) forwarderDelta {
	d := forwarderDelta{
		ErrorsByType:       make(map[string]int),
		HTTPErrorsByCode:   make(map[string]int),
		RequeuedByEndpoint: make(map[string]int),
		RetriedByEndpoint:  make(map[string]int),
		DroppedByEndpoint:  make(map[string]int),
	}

	d.Success = b.Transactions.Success - a.Transactions.Success
	d.Errors = b.Transactions.Errors - a.Transactions.Errors

	d.Dropped = b.Transactions.Dropped - a.Transactions.Dropped
	d.Retried = b.Transactions.Retried - a.Transactions.Retried
	d.Requeued = b.Transactions.Requeued - a.Transactions.Requeued

	d.RetryQueueSizeDelta = b.Transactions.RetryQueueSize - a.Transactions.RetryQueueSize
	d.HighPriorityQueueFull = b.Transactions.HighPriorityQueueFull - a.Transactions.HighPriorityQueueFull

	d.ConnectSuccess = b.Transactions.ConnectionEvents.ConnectSuccess - a.Transactions.ConnectionEvents.ConnectSuccess
	d.DNSSuccess = b.Transactions.ConnectionEvents.DNSSuccess - a.Transactions.ConnectionEvents.DNSSuccess

	for k, vb := range b.Transactions.ErrorsByType {
		d.ErrorsByType[k] = vb - a.Transactions.ErrorsByType[k]
	}
	for k, vb := range b.Transactions.HTTPErrorsByCode {
		d.HTTPErrorsByCode[k] = vb - a.Transactions.HTTPErrorsByCode[k]
	}
	for k, vb := range b.Transactions.RequeuedByEndpoint {
		d.RequeuedByEndpoint[k] = vb - a.Transactions.RequeuedByEndpoint[k]
	}
	for k, vb := range b.Transactions.RetriedByEndpoint {
		d.RetriedByEndpoint[k] = vb - a.Transactions.RetriedByEndpoint[k]
	}
	for k, vb := range b.Transactions.DroppedByEndpoint {
		d.DroppedByEndpoint[k] = vb - a.Transactions.DroppedByEndpoint[k]
	}

	return d
}

func apiKeyStatusDiagnosis(s forwarderSnapshot) diagnose.Diagnosis {
	if len(s.APIKeyStatus) == 0 {
		return diagnose.Diagnosis{
			Status:    diagnose.DiagnosisWarning,
			Name:      "Forwarder API key status",
			Category:  "forwarder",
			Diagnosis: "APIKeyStatus missing from forwarder expvar",
		}
	}

	keys := make([]string, 0, len(s.APIKeyStatus))
	for k := range s.APIKeyStatus {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	k := keys[0]
	v := s.APIKeyStatus[k]

	if v == "API Key valid" {
		return diagnose.Diagnosis{
			Status:    diagnose.DiagnosisSuccess,
			Name:      "Forwarder API key status",
			Category:  "forwarder",
			Diagnosis: fmt.Sprintf("%s: %s", k, v),
		}
	}

	return diagnose.Diagnosis{
		Status:      diagnose.DiagnosisWarning,
		Name:        "Forwarder API key status",
		Category:    "forwarder",
		Diagnosis:   fmt.Sprintf("%s: %s", k, v),
		Remediation: "If this says it cannot reach validation endpoint, treat as connectivity/proxy/firewall/TLS problem (not a bad key) and run the core connectivity suites.",
	}
}

func forwarderHealthDiagnosis(d forwarderDelta, window time.Duration) diagnose.Diagnosis {
	d.Window = window

	noActivity :=
		d.Success == 0 &&
			d.Errors == 0 &&
			d.Dropped == 0 &&
			d.Retried == 0 &&
			d.Requeued == 0 &&
			d.RetryQueueSizeDelta == 0 &&
			d.HighPriorityQueueFull == 0 &&
			sumMap(d.ErrorsByType) == 0 &&
			sumMap(d.HTTPErrorsByCode) == 0

	if noActivity {
		return diagnose.Diagnosis{
			Status:    diagnose.DiagnosisSuccess,
			Name:      "Forwarder health",
			Category:  "forwarder",
			Diagnosis: fmt.Sprintf("No forwarder activity observed over %s (no counter deltas).", window),
		}
	}

	dominantKey := ""
	dominantVal := 0
	errKeys := make([]string, 0, len(d.ErrorsByType))
	for k := range d.ErrorsByType {
		errKeys = append(errKeys, k)
	}
	sort.Strings(errKeys)
	for _, k := range errKeys {
		v := d.ErrorsByType[k]
		if v > dominantVal {
			dominantKey, dominantVal = k, v
		}
	}

	if d.Dropped > 0 || sumMap(d.DroppedByEndpoint) > 0 {
		return diagnose.Diagnosis{
			Status:      diagnose.DiagnosisFail,
			Name:        "Forwarder health",
			Category:    "forwarder",
			Description: "Detect data loss/backpressure/errors from forwarder expvar deltas",
			Diagnosis:   fmt.Sprintf("Data dropped in last %s (Dropped=%d).", window, d.Dropped),
			Remediation: "This indicates the Agent is losing payloads. Check network reachability, proxy/TLS, and consider tuning forwarder workers/timeouts if connectivity is flaky.",
		}
	}

	if dominantVal > 0 {
		status := diagnose.DiagnosisFail
		remediation := "Run `datadog-agent diagnose -v` suites connectivity-datadog-core-endpoints and connectivity-datadog-event-platform, then check proxy/TLS settings and outbound firewall rules."

		if len(d.HTTPErrorsByCode) > 0 && sumMap(d.HTTPErrorsByCode) > 0 {
			status = diagnose.DiagnosisWarning
			remediation = "HTTP errors suggest auth/permissions (401/403) or upstream/proxy issues (5xx). Verify api_key, site, and proxy configuration."
		}

		diag := fmt.Sprintf("Forwarder unhealthy over %s: Success=%d Errors=%d DominantError=%s(%d) Requeued=%d Retried=%d",
			window, d.Success, d.Errors, dominantKey, dominantVal, d.Requeued, d.Retried)

		md := map[string]string{
			"errors_by_type_delta":      mustJSON(d.ErrorsByType),
			"http_errors_by_code_delta": mustJSON(d.HTTPErrorsByCode),
		}

		return diagnose.Diagnosis{
			Status:      status,
			Name:        "Forwarder health",
			Category:    "forwarder",
			Description: "Detect data loss/backpressure/errors from forwarder expvar deltas",
			Diagnosis:   diag,
			Remediation: remediation,
			Metadata:    md,
		}
	}

	if d.Requeued > 0 || d.Retried > 0 || d.HighPriorityQueueFull > 0 {
		return diagnose.Diagnosis{
			Status:      diagnose.DiagnosisWarning,
			Name:        "Forwarder health",
			Category:    "forwarder",
			Diagnosis:   fmt.Sprintf("Forwarder seeing retries/requeues over %s (Requeued=%d Retried=%d HighPriorityQueueFull=%d).", window, d.Requeued, d.Retried, d.HighPriorityQueueFull),
			Remediation: "This is typically transient connectivity or backpressure. If persistent, review proxy/firewall and consider tuning forwarder settings.",
		}
	}

	return diagnose.Diagnosis{
		Status:    diagnose.DiagnosisSuccess,
		Name:      "Forwarder health",
		Category:  "forwarder",
		Diagnosis: fmt.Sprintf("Forwarder healthy over %s (Success=%d Errors=%d).", window, d.Success, d.Errors),
	}
}

func topEndpointsDiagnosis(d forwarderDelta, status diagnose.Status) diagnose.Diagnosis {
	type endpointCount struct {
		Endpoint string `json:"endpoint"`
		Count    int    `json:"count"`
	}

	tops := make([]endpointCount, 0, len(d.RequeuedByEndpoint))
	for endpoint, count := range d.RequeuedByEndpoint {
		if count > 0 {
			tops = append(tops, endpointCount{Endpoint: endpoint, Count: count})
		}
	}

	sort.Slice(tops, func(i, j int) bool { return tops[i].Count > tops[j].Count })
	if len(tops) > 5 {
		tops = tops[:5]
	}

	diagnosisText := "Top endpoints by requeue delta captured in metadata."
	if status != diagnose.DiagnosisSuccess {
		diagnosisText = "Forwarder is not healthy; endpoint impact details captured in metadata."
	}
	if len(tops) == 0 {
		if status != diagnose.DiagnosisSuccess {
			diagnosisText = "Forwarder is not healthy; no endpoint requeues observed in the sampling window."
		} else {
			diagnosisText = "No endpoint requeues observed in the sampling window."
		}
	}

	return diagnose.Diagnosis{
		Status:      status,
		Name:        "Forwarder top impacted endpoints",
		Category:    "forwarder",
		Description: "Endpoint-level requeue/retry deltas from forwarder expvar",
		Diagnosis:   diagnosisText,
		Metadata: map[string]string{
			"top_requeued_by_endpoint":   mustJSON(tops),
			"requeued_by_endpoint_delta": mustJSON(d.RequeuedByEndpoint),
			"retried_by_endpoint_delta":  mustJSON(d.RetriedByEndpoint),
			"dropped_by_endpoint_delta":  mustJSON(d.DroppedByEndpoint),
		},
	}
}

func sumMap(m map[string]int) int {
	s := 0
	for _, v := range m {
		s += v
	}
	return s
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	return string(b)
}
