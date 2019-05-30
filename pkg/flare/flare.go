// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	bodyReader, bodyWriter := io.Pipe()
	defer bodyReader.Close()
	writer := multipart.NewWriter(bodyWriter)

	//Write stuff to the pipe will block until it is read from the other end, so we don't load everything in memory
	go func() {

		defer bodyWriter.Close()
		defer writer.Close()

		if caseID != "" {
			writer.WriteField("case_id", caseID)
		}
		if email != "" {
			writer.WriteField("email", email)
		}

		//Flare file
		p, err := writer.CreateFormFile("flare_file", filepath.Base(archivePath))
		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		file, err := os.Open(archivePath)
		defer file.Close()
		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		_, err = io.Copy(p, file)
		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}

		agentFullVersion, _ := version.New(version.AgentVersion, version.Commit)
		writer.WriteField("agent_version", agentFullVersion.String())
		writer.WriteField("hostname", hostname)

	}()

	var url = mkURL(caseID)
	request, err := http.NewRequest("POST", url, bodyReader)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err != nil {
		return "", err
	}

	// As we use a PipeReader, we need to set the ContentLenght to -1 to have the correct Header in the POST.
	// see https://github.com/golang/go/issues/18117.
	request.ContentLength = -1

	client := mkHTTPClient()
	r, err := client.Do(request)

	return analyzeResponse(r, err)
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
	b, _ := ioutil.ReadAll(r.Body)
	var res = flareResponse{}
	err = json.Unmarshal(b, &res)
	if err != nil {
		response = fmt.Sprintf("An unknown error has occurred - Please contact support by email.")
		return response, err
	}

	if res.Error != "" {
		response = fmt.Sprintf("An error occurred while uploading the flare: %s. Please contact support by email.", res.Error)
		return response, fmt.Errorf("%s", res.Error)
	}

	response = fmt.Sprintf("Your logs were successfully uploaded. For future reference, your internal case id is %d", res.CaseID)
	return response, nil
}

func mkHTTPClient() *http.Client {
	transport := util.CreateHTTPTransport()

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
