// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apm fetch information about the apm agent.
// This will, in time, be migrated to the apm agent component.
package apm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// httpClients should be reused instead of created as needed. They keep cached TCP connections
// that may leak otherwise
var (
	httpClient     *http.Client
	clientInitOnce sync.Once
)

func client() *http.Client {
	clientInitOnce.Do(func() {
		httpClient = apiutil.GetClient(false)
	})

	return httpClient
}

// GetAPMStatus returns a set of key/value pairs summarizing the status of the trace-agent.
// If the status can not be obtained for any reason, the returned map will contain an "error"
// key with an explanation.
func GetAPMStatus() map[string]interface{} {
	port := config.Datadog.GetInt("apm_config.debug.port")

	c := client()
	url := fmt.Sprintf("http://localhost:%d/debug/vars", port)
	resp, err := apiutil.DoGet(c, url, apiutil.CloseConnection)
	if err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}

	status := make(map[string]interface{})
	if err := json.Unmarshal(resp, &status); err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}
	return status
}
