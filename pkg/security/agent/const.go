// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

var (
	// logType is the default Datadog Log source name used to ship security events
	logType string = "runtime-security"
	// logService is the default Datadog log service name used to ship security events
	logService string = "runtime-security-agent"
	// logSource is the default Datadog log source service name used to ship security events
	logSource string = "runtime-security-agent"
)
