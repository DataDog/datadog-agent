// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetProcessAgentStatus fetches the process-agent status from the process-agent API server
func GetProcessAgentStatus() map[string]interface{} {
	httpClient := apiutil.GetClient(false)

	s := make(map[string]interface{})
	addressPort, err := api.GetAPIAddressPort()
	if err != nil {
		s["error"] = fmt.Sprintf("%v", err.Error())
		return s
	}

	statusEndpoint := fmt.Sprintf("http://%s/agent/status", addressPort)
	b, err := apiutil.DoGet(httpClient, statusEndpoint)
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

// GetProcessAgentRuntimeConfig fetches the process-agent runtime settings.
// The API server in process-agent already scrubs and marshals the runtime settings as YAML.
// Since the api_key has been obfuscated with *, we're not able to unmarshal the response as YAML because *
// is not a valid YAML character
func GetProcessAgentRuntimeConfig(statusURL string) []byte {
	httpClient := apiutil.GetClient(false)

	b, err := apiutil.DoGet(httpClient, statusURL)
	if err != nil {
		return marshalError(fmt.Errorf("process-agent is not running or is unreachable"))
	}

	return b
}
