// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package apiserver

import klog "k8s.io/klog"

var supressedWarning = "v1 ComponentStatus is deprecated in v1.19+"

type CustomWarningLogger struct{}

// HandleWarningHeader suppresses some warning logs
// TODO: Remove custom warning logger when we remove usage of ComponentStatus
func (CustomWarningLogger) HandleWarningHeader(code int, agent string, message string) {
	if code != 299 || len(message) == 0 || message == supressedWarning {
		return
	}

	klog.Warning(message)
}
