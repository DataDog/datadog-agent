// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
)

// GetProcessAgentStatus returns the status command of the process-agent
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
