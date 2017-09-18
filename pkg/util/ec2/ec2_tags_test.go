// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build ec2

package ec2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetIAMRole(t *testing.T) {
	expected := "test-role"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := getIAMRole()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/iam/security-credentials/")
}

func TestGetSecurityCreds(t *testing.T) {
	expected1 := "test-role"
	expected2 := map[string]string{
		"Code":            "Success",
		"LastUpdated":     "2017-09-06T19:18:06Z",
		"Type":            "AWS-HMAC",
		"AccessKeyId":     "123123",
		"SecretAccessKey": "ddddd",
		"Token":           "asdfasdf",
		"Expiration":      "2017-09-07T01:45:43Z",
	}
	var lastRequest *http.Request
	var requestNumber = 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestNumber == 0 {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, expected1)
		} else {
			w.Header().Set("Content-Type", "text/plain")
			b, err := json.Marshal(expected2)
			if err != nil {
				assert.Fail(t, fmt.Sprintf("failed to marshall the map into json: %v", err))
			}
			io.WriteString(w, string(b))
		}
		requestNumber++
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := getSecurityCreds()
	assert.Nil(t, err)
	assert.Equal(t, expected2, val)
	assert.Equal(t, lastRequest.URL.Path, "/iam/security-credentials/test-role/")
}

func TestGetInstanceIdentity(t *testing.T) {
	expected := map[string]string{
		"privateIp":          "172.21.21.184",
		"devpayProductCodes": "",
		"availabilityZone":   "us-east-1a",
		"version":            "2010-08-31",
		"region":             "us-east-1",
		"instanceId":         "i-07d4be5da8ffb9d1b",
		"billingProducts":    "",
		"instanceType":       "c3.xlarge",
		"accountId":          "727006795293",
		"architecture":       "x86_64",
		"kernelId":           "",
		"ramdiskId":          "",
		"imageId":            "ami-ca5003dd",
		"pendingTime":        "2017-05-22T15:15:20Z",
	}
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		b, err := json.Marshal(expected)
		if err != nil {
			assert.Fail(t, fmt.Sprintf("failed to marshall the map into json: %v", err))
		}
		io.WriteString(w, string(b))
		lastRequest = r
	}))
	defer ts.Close()
	instanceIdentityURL = ts.URL

	val, err := getInstanceIdentity()
	assert.Nil(t, err)
	assert.Equal(t, expected, val)
}
