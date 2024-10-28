// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

// TraceEvent contains the raw event as well as the contents of
// every field as string, as defined under "Output format" in
// https://www.kernel.org/doc/Documentation/trace/ftrace.txt
type TraceEvent struct {
	Raw       string
	Task      string
	PID       uint32
	CPU       string
	Flags     string
	Timestamp string
	Function  string
	Message   string
}

func (t TraceEvent) String() string {
	return t.Raw
}
