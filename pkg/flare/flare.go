// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var datadogSupportURL = "/support/flare"
var httpTimeout = time.Duration(60)

type flareResponse struct {
	CaseID int    `json:"case_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// SendFlareWithHostname sends a flare with a set hostname
func SendFlareWithHostname(archivePath string, caseID string, email string, hostname string) (string, error) {
	r, err := readAndPostFlareFile(archivePath, caseID, email, hostname)
	return analyzeResponse(r, err)
}

func getFlareReader(multipartBoundary, archivePath, caseID, email, hostname string) io.ReadCloser {
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

		p, err := writer.CreateFormFile("flare_file", filepath.Base(archivePath))
		if err != nil {
			bodyWriter.CloseWithError(err) //nolint:errcheck
			return
		}
		file, err := os.Open(archivePath)
		defer file.Close()
		if err != nil {
			bodyWriter.CloseWithError(err) //nolint:errcheck
			return
		}
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

func readAndPostFlareFile(archivePath, caseID, email, hostname string) (*http.Response, error) {

	var url = mkURL(caseID)

	request, err := http.NewRequest("POST", url, nil) //nil body, we set it manually later
	if err != nil {
		return nil, err
	}

	// We need to set the Content-Type header here, but we still haven't created the writer
	// to obtain it from. Here we create one which only purpose is to give us a proper
	// Content-Type. Note that this Content-Type header will contain a random multipart
	// boundary, so we need to make sure that the actual writter uses the same boundary.
	boundaryWriter := multipart.NewWriter(nil)
	request.Header.Set("Content-Type", boundaryWriter.FormDataContentType())

	// Manually set the Body and ContentLenght. http.NewRequest doesn't do all of this
	// for us, since a PipeReader is not one of the Reader types it knows how to handle.
	request.Body = getFlareReader(boundaryWriter.Boundary(), archivePath, caseID, email, hostname)
	// -1 here means 'unknown' and makes this a 'chunked' request. See https://github.com/golang/go/issues/18117
	request.ContentLength = -1
	// Setting a GetBody function makes the request replayable in case there is a redirect.
	// Otherwise, since the body is a pipe, what has been already read can't be read again.
	request.GetBody = func() (io.ReadCloser, error) {
		return getFlareReader(boundaryWriter.Boundary(), archivePath, caseID, email, hostname), nil
	}

	client := mkHTTPClient()
	return client.Do(request)
}

// SendFlare will send a flare and grab the local hostname
func SendFlare(archivePath string, caseID string, email string) (string, error) {
	hostname, err := util.GetHostname()
	if err != nil {
		hostname = "unknown"
	}
	return SendFlareWithHostname(archivePath, caseID, email, hostname)
}

func analyzeResponse(r *http.Response, err error) (string, error) {
	var response string
	if err != nil {
		return response, err
	}
	if r.StatusCode == http.StatusForbidden {
		apiKey := config.Datadog.GetString("api_key")
		var errStr string

		if len(apiKey) == 0 {
			errStr = "API key is missing"
		} else {
			if len(apiKey) > 5 {
				apiKey = apiKey[len(apiKey)-5:]
			}
			errStr = fmt.Sprintf("Make sure your API key is valid. API Key ending with: %v", apiKey)
		}

		return response, fmt.Errorf("HTTP 403 Forbidden: %s", errStr)
	}

	b, _ := ioutil.ReadAll(r.Body)
	var res = flareResponse{}
	err = json.Unmarshal(b, &res)
	if err != nil {
		response = fmt.Sprintf("Error: could not deserialize response body -- Please contact support by email.")
		return response, fmt.Errorf("%v\nServer returned:\n%s", err, string(b)[:150])
	}

	if res.Error != "" {
		response = fmt.Sprintf("An error occurred while uploading the flare: %s. Please contact support by email.", res.Error)
		return response, fmt.Errorf("%s", res.Error)
	}

	response = fmt.Sprintf("Your logs were successfully uploaded. For future reference, your internal case id is %d", res.CaseID)
	return response, nil
}

func mkHTTPClient() *http.Client {
	transport := httputils.CreateHTTPTransport()

	client := &http.Client{
		Transport: transport,
		Timeout:   httpTimeout * time.Second,
	}

	return client
}

func mkURL(caseID string) string {
	baseURL, _ := config.AddAgentVersionToDomain(config.GetMainInfraEndpoint(), "flare")
	var url = baseURL + datadogSupportURL
	if caseID != "" {
		url += "/" + caseID
	}
	url += "?api_key=" + config.Datadog.GetString("api_key")
	return url
}
