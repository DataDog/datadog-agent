// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build ec2

package ec2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"sync"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type requestRecorder struct {
	lastRequest    *http.Request
	requestCounter int
	mutex          sync.RWMutex
}

func (l *requestRecorder) recordRequest(r *http.Request) {
	l.mutex.Lock()
	l.lastRequest = r
	l.requestCounter++
	l.mutex.Unlock()
}

func (l *requestRecorder) getRequestCounter() int {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.requestCounter
}

func (l *requestRecorder) getRequestPath() string {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.lastRequest == nil {
		return ""
	}
	return l.lastRequest.URL.Path
}

func TestGetIAMRole(t *testing.T) {
	const expected = "test-role"
	rr := &requestRecorder{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		rr.recordRequest(r)
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := getIAMRole()
	require.Nil(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, rr.getRequestPath(), "/iam/security-credentials/")
}

func TestGetSecurityCreds(t *testing.T) {
	const expected1 = "test-role"
	expected2Lock := sync.RWMutex{}
	expected2 := map[string]string{
		"Code":            "Success",
		"LastUpdated":     "2017-09-06T19:18:06Z",
		"Type":            "AWS-HMAC",
		"AccessKeyId":     "123123",
		"SecretAccessKey": "ddddd",
		"Token":           "asdfasdf",
		"Expiration":      "2017-09-07T01:45:43Z",
	}
	rr := &requestRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rr.getRequestCounter() == 0 {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, expected1)
		} else {
			w.Header().Set("Content-Type", "text/plain")
			expected2Lock.Lock()
			b, err := json.Marshal(expected2)
			expected2Lock.Unlock()
			require.Nil(t, err, fmt.Sprintf("failed to marshall the map into json: %v", err))
			io.WriteString(w, string(b))
		}
		rr.recordRequest(r)
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := getSecurityCreds()
	require.Nil(t, err)
	expected2Lock.Lock()
	assert.Equal(t, expected2, val)
	expected2Lock.Unlock()
	assert.Equal(t, rr.getRequestPath(), "/iam/security-credentials/test-role/")
}

func TestGetInstanceIdentity(t *testing.T) {
	expectedLock := sync.RWMutex{}
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
	rr := &requestRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		expectedLock.Lock()
		b, err := json.Marshal(expected)
		expectedLock.Unlock()
		require.Nil(t, err, fmt.Sprintf("failed to marshall the map into json: %v", err))
		io.WriteString(w, string(b))
		rr.recordRequest(r)
	}))
	defer ts.Close()
	instanceIdentityURL = ts.URL

	val, err := getInstanceIdentity()
	require.Nil(t, err)
	expectedLock.RLock()
	assert.Equal(t, expected, val)
	expectedLock.RUnlock()
}
