// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestConfigSetHandler(t *testing.T) {
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

	resp, err := r.DoReq(fmt.Sprintf("config/set?log_level=%s", seelog.WarnStr), http.MethodPost, nil)
	if err != nil {
		t.Fatal(err)
	}

	logstr = r.AgentLog()
	assert.NotContains(t, logstr, "Switched log level to")
	assert.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
}
