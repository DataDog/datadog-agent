// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	datadogSupportURL        = "/support/flare"
	datadogSupportAnalyzeURL = "/support/flare/analyze"
	httpTimeout              = time.Duration(60) * time.Second
)

// any modification to this struct should also be applied to datadog-agent/test/fakeintake/server/body.go
type flareResponse struct {
	CaseID      int    `json:"case_id,omitempty"`
	Error       string `json:"error,omitempty"`
	RequestUUID string `json:"request_uuid,omitempty"`
	JiraTicket  string `json:"jira_ticket,omitempty"`
}

// FlareSource has metadata about why the flare was sent
type FlareSource struct {
	sourceType string
	rcTaskUUID string
}

// NewLocalFlareSource returns a flare source struct for local flares
func NewLocalFlareSource() FlareSource {
	return FlareSource{
		sourceType: "local",
	}
}

// NewRemoteConfigFlareSource returns a flare source struct for remote-config
func NewRemoteConfigFlareSource(rcTaskUUID string) FlareSource {
	return FlareSource{
		sourceType: "remote-config",
		rcTaskUUID: rcTaskUUID,
	}
}

func getFlareReader(multipartBoundary, archivePath, caseID, email, hostname string, source FlareSource) io.ReadCloser {
	//No need to close the reader, http.Client does it for us
	bodyReader, bodyWriter := io.Pipe()

	writer := multipart.NewWriter(bodyWriter)
	writer.SetBoundary(multipartBoundary) //nolint:errcheck

	//Write stuff to the pipe will block until it is read from the other end, so we don't load everything in memory
	go func() {
		// defer order matters to avoid empty result when reading the form.
		defer bodyWriter.Close()
		defer writer.Close()

		if caseID != "" {
			writer.WriteField("case_id", caseID) //nolint:errcheck
		}
		if email != "" {
			writer.WriteField("email", email) //nolint:errcheck
		}
		if source.sourceType != "" {
			writer.WriteField("source", source.sourceType) //nolint:errcheck
		}
		if source.rcTaskUUID != "" {
			// UUID of the remote-config task sending the flare
			writer.WriteField("rc_task_uuid", source.rcTaskUUID) //nolint:errcheck
		}

		p, err := writer.CreateFormFile("flare_file", filepath.Base(archivePath))
		if err != nil {
			bodyWriter.CloseWithError(err) //nolint:errcheck
			return
		}
		file, err := os.Open(archivePath)
		if err != nil {
			bodyWriter.CloseWithError(err) //nolint:errcheck
			return
		}
		defer file.Close()

		_, err = io.Copy(p, file)
		if err != nil {
			bodyWriter.CloseWithError(err) //nolint:errcheck
			return
		}

		agentFullVersion, _ := version.Agent()
		writer.WriteField("agent_version", agentFullVersion.String()) //nolint:errcheck
		writer.WriteField("hostname", hostname)                       //nolint:errcheck
	}()

	return bodyReader
}

func readAndPostFlareFile(archivePath, caseID, email, hostname, url string, source FlareSource, client *http.Client, apiKey string) (*http.Response, error) {
	// Having resolved the POST URL, we do not expect to see further redirects, so do not
	// handle them.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	request, err := http.NewRequest("POST", url, nil) //nil body, we set it manually later
	if err != nil {
		return nil, err
	}
	request.Header.Add("DD-API-KEY", apiKey)

	// We need to set the Content-Type header here, but we still haven't created the writer
	// to obtain it from. Here we create one which only purpose is to give us a proper
	// Content-Type. Note that this Content-Type header will contain a random multipart
	// boundary, so we need to make sure that the actual writter uses the same boundary.
	boundaryWriter := multipart.NewWriter(nil)
	request.Header.Set("Content-Type", boundaryWriter.FormDataContentType())

	// Manually set the Body and ContentLenght. http.NewRequest doesn't do all of this
	// for us, since a PipeReader is not one of the Reader types it knows how to handle.
	request.Body = getFlareReader(boundaryWriter.Boundary(), archivePath, caseID, email, hostname, source)

	// -1 here means 'unknown' and makes this a 'chunked' request. See https://github.com/golang/go/issues/18117
	request.ContentLength = -1

	resp, err := client.Do(request)
	if err != nil {
		return resp, err
	}

	// Convert 5xx HTTP error status codes to Go errors for retry logic
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, fmt.Errorf("HTTP %d %s\nServer returned:\n%s", resp.StatusCode, http.StatusText(resp.StatusCode), string(body))
	}

	return resp, nil
}

func analyzeResponse(r *http.Response, apiKey string) (string, error) {
	if r.StatusCode == http.StatusForbidden {
		var errStr string

		if len(apiKey) == 0 {
			errStr = "API key is missing"
		} else {
			if len(apiKey) > 5 {
				apiKey = apiKey[len(apiKey)-5:]
			}
			errStr = fmt.Sprintf("Make sure your API key is valid. API Key ending with: %v", apiKey)
		}

		return "", fmt.Errorf("HTTP 403 Forbidden: %s", errStr)
	}

	res := flareResponse{}

	var err error
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != http.StatusOK {
		err = fmt.Errorf("HTTP %d %s", r.StatusCode, r.Status)
	} else if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		if contentType != "" {
			err = fmt.Errorf("Server returned a %d but with an unknown content-type %s", http.StatusOK, contentType)
		} else {
			err = fmt.Errorf("Server returned a %d but with no content-type header", http.StatusOK)
		}
	} else {
		err = json.Unmarshal(b, &res)
	}

	if err != nil {
		response := "Error: could not deserialize response body -- Please contact support by email."
		sample := string(b)
		if len(sample) > 150 {
			sample = sample[:150]
		}
		return response, fmt.Errorf("%v\nServer returned:\n%s", err, sample)
	}

	if res.Error != "" {
		var uuidReport string
		if res.RequestUUID != "" {
			uuidReport = fmt.Sprintf(" and facilitate the request uuid: `%s`", res.RequestUUID)
		}
		response := fmt.Sprintf("An error occurred while uploading the flare: %s. Please contact support by email%s.", res.Error, uuidReport)
		return response, errors.New(res.Error)
	}

	// If detecting false positives along with the flare, also link the jira ticket
	response := fmt.Sprintf("Your logs were successfully uploaded. For future reference, your internal case id is %d", res.CaseID)
	if res.JiraTicket != "" {
		response += fmt.Sprintf("\nFollow this Jira Ticket for process false positive detection results: %s", res.JiraTicket)
	}

	return response, nil
}

// Resolve a flare URL to the URL at which a POST should be made.  This uses a HEAD request
// to follow any redirects, avoiding the problematic behavior of a POST that results in a
// redirect (and often in an early termination of the connection).
func resolveFlarePOSTURL(url string, client *http.Client, apiKey string) (string, error) {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Add("DD-API-KEY", apiKey)

	r, err := client.Do(request)
	if err != nil {
		return "", err
	}

	defer r.Body.Close()
	// at the end of the chain of redirects, we should either have a 200 OK or a 404 (since
	// the server is expecting POST, not GET). Accept either one as successful.
	if r.StatusCode != http.StatusOK && r.StatusCode != http.StatusNotFound {
		return "", fmt.Errorf("We couldn't reach the flare backend %s via redirects: %s", scrubber.ScrubLine(url), r.Status)
	}

	// return the URL used to make the latest request (at the end of the chain of redirects)
	return r.Request.URL.String(), nil
}

func mkURL(baseURL string, caseID string) string {
	url := baseURL + datadogSupportURL
	if caseID != "" {
		url += "/" + caseID
	}
	return url
}

// SendTo sends a flare file to the backend. This is part of the "helpers" package while all the code is moved to
// components. When possible use the "Send" method of the "flare" component instead.
func SendTo(cfg pkgconfigmodel.Reader, archivePath, caseID, email, apiKey, url string, source FlareSource) (string, error) {
	hostname, err := hostnameUtil.Get(context.TODO())
	if err != nil {
		hostname = "unknown"
	}

	apiKey = configUtils.SanitizeAPIKey(apiKey)
	baseURL, _ := configUtils.AddAgentVersionToDomain(url, "flare")

	transport := httputils.CreateHTTPTransport(cfg)
	client := &http.Client{
		Transport: transport,
		Timeout:   httpTimeout,
	}

	url = mkURL(baseURL, caseID)

	url, err = resolveFlarePOSTURL(url, client, apiKey)
	if err != nil {
		return "", err
	}

	// Retry logic for the actual flare file posting
	var lastErr error
	var baseDelay = 1 * time.Second

	for attempt := 3; attempt > 0; attempt-- {
		r, err := readAndPostFlareFile(archivePath, caseID, email, hostname, url, source, client, apiKey)
		if err != nil {
			// Always close the response body if it exists
			statusCode := 0
			if r != nil {
				statusCode = r.StatusCode
				r.Body.Close()
			}
			lastErr = err

			if !isRetryableFlareError(err, statusCode) {
				return "", err
			}
			log.Warn("Failed to send flare, retrying in 1 second")
			time.Sleep(baseDelay)
			continue
		}

		// Success case - analyze the response
		defer r.Body.Close()
		return analyzeResponse(r, apiKey)
	}
	return "", fmt.Errorf("failed to send flare after 3 attempts: %w", lastErr)
}

// SendToAnalyze sends a flare file to the analyze endpoint for false positive detection.
func SendToAnalyze(cfg pkgconfigmodel.Reader, archivePath, caseID, email, apiKey, url string, source FlareSource) (string, error) {
	hostname, err := hostnameUtil.Get(context.TODO())
	if err != nil {
		hostname = "unknown"
	}

	apiKey = configUtils.SanitizeAPIKey(apiKey)

	// Check for analyze-specific URL override
	// This is used to allow for overriding to local host so that we can test the analyze endpoint without having to deploy any services to Datadog.
	analyzeBaseURL := url
	if cfg.IsConfigured("flare_analyze_url") && cfg.GetString("flare_analyze_url") != "" {
		analyzeBaseURL = cfg.GetString("flare_analyze_url")
	}

	baseURL, _ := configUtils.AddAgentVersionToDomain(analyzeBaseURL, "flare")

	transport := httputils.CreateHTTPTransport(cfg)
	client := &http.Client{
		Transport: transport,
		Timeout:   httpTimeout,
	}

	// Use the analyze endpoint instead of the regular flare endpoint
	analyzeURL := baseURL + datadogSupportAnalyzeURL
	if caseID != "" {
		analyzeURL += "/" + caseID
	}

	analyzeURL, err = resolveFlarePOSTURL(analyzeURL, client, apiKey)
	if err != nil {
		return "", err
	}

	// Retry logic for the actual flare file posting
	var lastErr error
	var baseDelay = 1 * time.Second

	for attempt := 3; attempt > 0; attempt-- {
		r, err := readAndPostFlareFile(archivePath, caseID, email, hostname, analyzeURL, source, client, apiKey)
		if err != nil {
			// Always close the response body if it exists
			statusCode := 0
			if r != nil {
				statusCode = r.StatusCode
				r.Body.Close()
			}
			lastErr = err

			if !isRetryableFlareError(err, statusCode) {
				return "", err
			}
			log.Warn("Failed to send flare to analyze endpoint, retrying in 1 second")
			time.Sleep(baseDelay)
			continue
		}

		// Success case - analyze the response
		defer r.Body.Close()
		return analyzeResponse(r, apiKey)
	}
	return "", fmt.Errorf("failed to send flare to analyze endpoint after 3 attempts: %w", lastErr)
}

func isRetryableFlareError(err error, statusCode int) bool {
	if err == nil {
		return false
	}

	if statusCode >= 500 && statusCode < 600 {
		return true
	}

	errStr := strings.ToLower(err.Error())

	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "network unreachable") ||
		strings.Contains(errStr, "temporary failure")
}

// GetFlareEndpoint creates the flare endpoint URL
func GetFlareEndpoint(cfg config.Reader) string {
	// Create flare endpoint to the shape of "https://<version>-flare.agent.datadoghq.com/support/flare"
	flareRoute, _ := configUtils.AddAgentVersionToDomain(configUtils.GetInfraEndpoint(cfg), "flare")
	return flareRoute + datadogSupportURL
}

// SendFlare sends a flare and returns the message returned by the backend. This entry point is deprecated in favor of
// the 'Send' method of the flare component.
func SendFlare(cfg pkgconfigmodel.Reader, archivePath string, caseID string, email string, source FlareSource) (string, error) {
	return SendTo(cfg, archivePath, caseID, email, cfg.GetString("api_key"), configUtils.GetInfraEndpoint(cfg), source)
}
