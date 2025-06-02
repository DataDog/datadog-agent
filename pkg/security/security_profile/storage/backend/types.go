// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package backend holds files related to forwarder backends for security profiles
package backend

// ActivityDumpHandler represents an handler for the activity dumps sent by the probe
type ActivityDumpHandler interface {
	HandleActivityDump(imageName string, imageTag string, header []byte, data []byte) error
}
