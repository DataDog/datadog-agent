// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestUserAgent(t *testing.T) {
	assert := assert.New(t)
	agentConfig := mock.New(t)

	// TLS test uses bogus certs
	agentConfig.SetWithoutSource("skip_ssl_validation", true)                    // Transport
	agentConfig.SetWithoutSource("remote_configuration.no_tls_validation", true) // RC check
	agentConfig.SetWithoutSource("remote_configuration.no_tls", true)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	userAgentCh := make(chan string, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		userAgentCh <- r.UserAgent()
	}))
	defer ts.Close()

	url, err := url.Parse(ts.URL)
	assert.NoError(err)

	client, err := NewHTTPClient(Auth{}, agentConfig, url)
	assert.NoError(err)

	_, _ = client.FetchOrgData(ctx)

	select {
	case ua := <-userAgentCh:
		// Regex explained:
		//   * ^datadog-agent\/ == must start with "datadog-agent/"
		//   * (unknown|\d+\.\d+\.\d+) == either "unknown" or a semver string
		//   * \(go\d+\.\d+\.\d+\)$ == ends in " (go1.2.3)" where 1.2.3 is a semver string
		uaRegex := regexp.MustCompile(`^datadog-agent\/(.+) \(go\d+\.\d+\.\d+\)$`)
		parts := uaRegex.FindStringSubmatch(ua)
		assert.Len(parts, 2) // Original string + the extracted group.

		// The extracted string must match either "unknown" or be valid semver.
		switch parts[1] {
		case "unknown":
		default:
			_, err = semver.NewVersion(parts[1])
			assert.NoError(err)
		}

	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for user agent string")
	}
}
