// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build ec2

package ec2

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIAMRole(t *testing.T) {
	const expected = "test-role"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, expected)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := getIAMRole()
	require.Nil(t, err)
	assert.Equal(t, expected, val)
}

func TestGetSecurityCreds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "test-role")
		} else if r.URL.Path == "/iam/security-credentials/test-role" {
			w.Header().Set("Content-Type", "text/plain")
			content, err := ioutil.ReadFile("payloads/security_cred.json")
			require.Nil(t, err, fmt.Sprintf("failed to load json in payloads/security_cred.json: %v", err))
			io.WriteString(w, string(content))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	metadataURL = ts.URL

	cred, err := getSecurityCreds()
	require.Nil(t, err)
	assert.Equal(t, "123456", cred.AccessKeyId)
	assert.Equal(t, "secret access key", cred.SecretAccessKey)
	assert.Equal(t, "secret token", cred.Token)
}

func TestGetInstanceIdentity(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		content, err := ioutil.ReadFile("payloads/instance_indentity.json")
		require.Nil(t, err, fmt.Sprintf("failed to load json in payloads/instance_indentity.json: %v", err))
		io.WriteString(w, string(content))
	}))
	defer ts.Close()
	instanceIdentityURL = ts.URL

	val, err := getInstanceIdentity()
	require.Nil(t, err)
	assert.Equal(t, "us-east-1", val.Region)
	assert.Equal(t, "i-aaaaaaaaaaaaaaaaa", val.InstanceId)
}
