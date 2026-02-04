// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// mockLogView implements observer.LogView for testing.
type mockLogView struct {
	content   []byte
	status    string
	tags      []string
	hostname  string
	timestamp int64
}

func (m *mockLogView) GetContent() []byte   { return m.content }
func (m *mockLogView) GetStatus() string    { return m.status }
func (m *mockLogView) GetTags() []string    { return m.tags }
func (m *mockLogView) GetHostname() string  { return m.hostname }
func (m *mockLogView) GetTimestamp() int64  { return m.timestamp }
