// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

import "fmt"

type logger struct{}

func (*logger) Trace(v ...interface{})                      {}
func (*logger) Tracef(format string, params ...interface{}) {}
func (*logger) Debug(v ...interface{})                      {}
func (*logger) Debugf(format string, params ...interface{}) {}
func (*logger) Info(v ...interface{}) {
	fmt.Println(v...)
}
func (*logger) Infof(format string, params ...interface{}) {}
func (*logger) Warn(v ...interface{}) error {
	return nil
}
func (*logger) Warnf(format string, params ...interface{}) error {
	return nil
}
func (*logger) Error(v ...interface{}) error {
	fmt.Println(v...)
	return nil
}
func (*logger) Errorf(format string, params ...interface{}) error {
	return nil
}
func (*logger) Critical(v ...interface{}) error {
	return nil
}
func (*logger) Criticalf(format string, params ...interface{}) error {
	return nil
}
func (*logger) Flush() {}
