// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package util

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

// GetClient is a convenience function returning an http client
// `GetClient(false)` must be used only for HTTP requests whose destination is
// localhost (ie, for Agent commands).
func GetClient(verify bool) *http.Client {
	if verify {
		return &http.Client{}
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	return &http.Client{Transport: tr}
}

// DoGet is a wrapper around performing HTTP GET requests
func DoGet(c *http.Client, url string) (body []byte, e error) {
	req, e := http.NewRequest("GET", url, nil)
	if e != nil {
		return body, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+GetAuthToken())

	r, e := c.Do(req)
	if e != nil {
		return body, e
	}
	body, e = ioutil.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return body, e
	}
	if r.StatusCode >= 400 {
		return body, fmt.Errorf("%s", body)
	}
	return body, nil

}

// DoPost is a wrapper around performing HTTP POST requests
func DoPost(c *http.Client, url string, contentType string, body io.Reader) (resp []byte, e error) {
	req, e := http.NewRequest("POST", url, body)
	if e != nil {
		return resp, e
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+GetAuthToken())

	r, e := c.Do(req)
	if e != nil {
		return resp, e
	}
	resp, e = ioutil.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return resp, e
	}
	if r.StatusCode >= 400 {
		return resp, fmt.Errorf("%s", resp)
	}
	return resp, nil
}

// DoPostChunked is a wrapper around performing HTTP POST requests that stream chunked data
func DoPostChunked(c *http.Client, url string, contentType string, body io.Reader, onChunk func([]byte)) error {
	req, e := http.NewRequest("POST", url, body)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+GetAuthToken())

	r, e := c.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()

	var m int
	buf := make([]byte, 4096)
	for {
		m, e = r.Body.Read(buf)
		if m < 0 || e != nil {
			break
		}
		onChunk(buf[:m])
	}

	if r.StatusCode == 200 {
		return nil
	}
	return e
}
