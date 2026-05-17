// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

package creds

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ec2internal "github.com/DataDog/datadog-agent/pkg/util/aws/creds/internal"
)

func TestGetIAMRole(t *testing.T) {
	ctx := context.Background()
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
	ec2internal.MetadataURL = ts.URL

	val, err := getIAMRole(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
}

func TestGetSecurityCredentials(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iam/security-credentials/" {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "test-role")
		} else if r.URL.Path == "/iam/security-credentials/test-role" {
			w.Header().Set("Content-Type", "text/plain")
			content, err := os.ReadFile("tags/payloads/security_cred.json")
			require.NoError(t, err, fmt.Sprintf("failed to load json in tags/payloads/security_cred.json: %v", err))
			w.Write(content)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL

	cred, err := GetSecurityCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, "123456", cred.AccessKeyID)
	assert.Equal(t, "secret access key", cred.SecretAccessKey)
	assert.Equal(t, "secret token", cred.Token)
}
