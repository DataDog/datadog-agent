// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processagent fetch information about the process agent
package processagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"gopkg.in/yaml.v2"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// httpClients should be reused instead of created as needed. They keep cached TCP connections
// that may leak otherwise
var (
	httpClient     *http.Client
	clientInitOnce sync.Once
)

func getHTTPClient() *http.Client {
	clientInitOnce.Do(func() {
		httpClient = apiutil.GetClient(false)
	})

	return httpClient
}

// GetStatus fetches the process-agent status from the process-agent API server
func GetStatus() map[string]interface{} {
	s := make(map[string]interface{})
	addressPort, err := config.GetProcessAPIAddressPort()
	if err != nil {
		s["error"] = fmt.Sprintf("%v", err.Error())
		return s
	}

	client := getHTTPClient()
	statusEndpoint := fmt.Sprintf("http://%s/agent/status", addressPort)
	b, err := apiutil.DoGet(client, statusEndpoint, apiutil.CloseConnection)
	if err != nil {
		s["error"] = fmt.Sprintf("%v", err.Error())
		return s
	}

	err = json.Unmarshal(b, &s)
	if err != nil {
		s["error"] = fmt.Sprintf("%v", err.Error())
		return s
	}

	return s
}

// marshalError marshals an error as YAML
func marshalError(err error) []byte {
	errYaml := map[string]string{
		"error": err.Error(),
	}

	b, err := yaml.Marshal(errYaml)
	if err != nil {
		log.Warn("Unable to marshal error as yaml")
		return nil
	}

	return b
}

// GetRuntimeConfig fetches the process-agent runtime settings.
// The API server in process-agent already scrubs and marshals the runtime settings as YAML.
// Since the api_key has been obfuscated with *, we're not able to unmarshal the response as YAML because *
// is not a valid YAML character
func GetRuntimeConfig(statusURL string) []byte {
	client := getHTTPClient()
	b, err := apiutil.DoGet(client, statusURL, apiutil.CloseConnection)
	if err != nil {
		return marshalError(fmt.Errorf("process-agent is not running or is unreachable"))
	}

	return b
}
