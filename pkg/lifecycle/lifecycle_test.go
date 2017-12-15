// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package lifecycle

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func assertServiceIs(t *testing.T, serverURL string, healthy bool) {
	r, err := http.Get(fmt.Sprintf("%s/health", serverURL))
	assert.Nil(t, err)
	content, err := ioutil.ReadAll(r.Body)
	assert.Nil(t, err)
	health := &lifecycleResponse{}
	err = json.Unmarshal(content, health)
	assert.Nil(t, err)
	assert.True(t, health.Health == healthy)
	if health.Health == true {
		assert.True(t, r.StatusCode == http.StatusOK)
	} else {
		assert.True(t, r.StatusCode == http.StatusServiceUnavailable)
	}
}

func TestLifecycle(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	RecordHealthPath()
	assertServiceIs(t, server.URL, false)

	l := GetLifecycle()
	// Pass the service OK then Unavailable several times
	for r := 0; r < 5; r++ {
		l.RefreshHealthStatus()
		assertServiceIs(t, server.URL, true)

		l.lastHealthRefresh = time.Now().Unix() - (expectedRefreshSeconds + 1)
		assertServiceIs(t, server.URL, false)
	}
}
