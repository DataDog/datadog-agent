// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sapabap

import "os"

// readEnv reads a deploy-time value from the process environment. The developer
// exports the Docker Hub credentials (and any optional SAPControl auth) before
// running the create task:
//
//	export DOCKERHUB_USER=<hub user>
//	export DOCKERHUB_TOKEN=<hub access token>   # EULA-accepted account
//	dda inv aws.integrations.sap_abap.create
//
// These values flow through to the Pulumi subprocess environment and are passed
// to the host only via the command Environment; they are never written to disk,
// inlined into a command string, or committed.
func readEnv(name string) string {
	return os.Getenv(name)
}
