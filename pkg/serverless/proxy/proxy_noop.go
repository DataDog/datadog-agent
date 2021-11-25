// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !serverlessexperimental

package proxy

// Start is a no-op operation, used in the classic serverless build (non experimental)
func Start(listenUrl string, originalUrl string) bool {
	// no-op
	return false
}
