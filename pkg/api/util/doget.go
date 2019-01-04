// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package util

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

// GetClient is a convenience function returning an http client
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
