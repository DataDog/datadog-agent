// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sapalm

import "os"

// readAPIKey reads the SAP Business Accelerator Hub APIKey from the process
// environment. The developer exports it before running the create task:
//
//	export SAP_ALM_API_KEY=<hub api key>
//	dda inv aws.integrations.sap_alm.create
//
// The value flows through to the Pulumi subprocess environment and is injected
// into the Agent container; it is never written to disk or committed.
func readAPIKey() string {
	return os.Getenv(apiKeyEnvVar)
}
