// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const processCheckEndpoint = "/api/v1/collector"

// Endpoint is a single endpoint where process data will be submitted.
type Endpoint struct {
	APIKey   string
	Endpoint *url.URL
}

// GetCheckURL returns the URL string for a given agent check
func (e *Endpoint) GetCheckURL(checkPath string) string {
	// Make a copy of the URL
	checkURL := *e.Endpoint

	// This is to maintain backward compatibility with agents configured with the default collector endpoint:
	// process_dd_url: https://process.datadoghq.com/api/v1/collector
	if checkURL.Path == processCheckEndpoint {
		checkURL.Path = ""
	}

	// Finally, add the checkPath to the existing API Endpoint path.
	// This is done like so to support certain use-cases in which `process_dd_url` points to something
	// like a NGINX server proxying requests under a certain path (eg. https://proxy-host/process-agent)
	checkURL.Path = path.Join(checkURL.Path, checkPath)
	return checkURL.String()
}

type postResponse struct {
	msg model.Message
	err error
}

func errResponse(format string, a ...interface{}) postResponse {
	return postResponse{err: fmt.Errorf(format, a...)}
}

// Client holds the process http API client
type Client struct {
	http       http.Client
	ctxTimeout time.Duration
}

// NewClient returns an API http client
func NewClient(client http.Client, ctxTimeout time.Duration) Client {
	return Client{
		http:       client,
		ctxTimeout: ctxTimeout,
	}
}

// PostMessage allows to post
func (c *Client) PostMessage(endpoints []Endpoint, checkPath string, m model.MessageBody, headers map[string]string) []*model.CollectorStatus {
	msgType, err := model.DetectMessageType(m)
	if err != nil {
		log.Errorf("Unable to detect message type: %s", err)
		return []*model.CollectorStatus{}
	}

	body, err := model.EncodeMessage(model.Message{
		Header: model.MessageHeader{
			Version:  model.MessageV3,
			Encoding: model.MessageEncodingZstdPB,
			Type:     msgType,
		}, Body: m})
	if err != nil {
		log.Errorf("Unable to encode message: %s", err)
	}

	responses := make(chan postResponse)
	for _, ep := range endpoints {
		extraHeaders := map[string]string{
			"X-Dd-APIKey": ep.APIKey,
		}
		go c.postToAPI(ep.GetCheckURL(checkPath), body, responses, headers, extraHeaders)
	}

	// Wait for all responses to come back before moving on.
	statuses := make([]*model.CollectorStatus, 0, len(endpoints))
	for i := 0; i < len(endpoints); i++ {
		url := endpoints[i].Endpoint.String()
		res := <-responses
		if res.err != nil {
			log.Error(res.err)
			continue
		}

		r := res.msg
		switch r.Header.Type {
		case model.TypeResCollector:
			rm := r.Body.(*model.ResCollector)
			if len(rm.Message) > 0 {
				log.Errorf("Error in response from %s: %s", url, rm.Message)
			} else {
				statuses = append(statuses, rm.Status)
			}
		default:
			log.Errorf("Unexpected response type from %s: %d", url, r.Header.Type)
		}
	}
	return statuses
}

func (c *Client) postToAPI(url string, body []byte, responses chan postResponse, headers map[string]string, extraHeaders map[string]string) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		responses <- errResponse("could not create request to %s: %s", url, err)
		return
	}

	for k, v := range headers {
		req.Header.Add(k, v)
	}
	for k, v := range extraHeaders {
		req.Header.Add(k, v)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.ctxTimeout)
	defer cancel()
	req.WithContext(ctx)

	resp, err := c.http.Do(req)
	if err != nil {
		if isHTTPTimeout(err) {
			responses <- errResponse("Timeout detected on %s, %s", url, err)
		} else {
			responses <- errResponse("Error submitting payload to %s: %s", url, err)
		}
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 300 {
		responses <- errResponse("unexpected response from %s. Status: %s", url, resp.Status)
		io.Copy(ioutil.Discard, resp.Body)
		return
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		responses <- errResponse("could not decode response body from %s: %s", url, err)
		return
	}

	r, err := model.DecodeMessage(body)
	if err != nil {
		responses <- errResponse("could not decode message from %s: %s", url, err)
	}
	responses <- postResponse{r, err}
}

// IsTimeout returns true if the error is due to reaching the timeout limit on the http.client
func isHTTPTimeout(err error) bool {
	if netErr, ok := err.(interface {
		Timeout() bool
	}); ok && netErr.Timeout() {
		return true
	}
	return false
}
