// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestConfigSetHandlerUnauthenticated(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if err := r.RunAgent([]byte("log_level: info")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	defer r.KillAgent()
	logstr := r.AgentLog()
	assert.NotContains(t, logstr, "| DEBUG |")
	assert.Contains(t, logstr, "| INFO |")

	resp, err := r.DoReq("config/set?log_level="+log.WarnStr, http.MethodPost, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}
