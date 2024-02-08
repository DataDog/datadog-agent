// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tailers

import (
	"testing"

	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	assert "github.com/stretchr/testify/require"
)

type TestTailer1 struct {
	id   string
	info *status.InfoRegistry
}

func NewTestTailer1(id string) *TestTailer1 {
	return &TestTailer1{
		id:   id,
		info: status.NewInfoRegistry(),
	}
}

//nolint:revive // TODO(AML) Fix revive linter
func (t *TestTailer1) GetId() string {
	return t.id
}
func (t *TestTailer1) GetType() string {
	return "test"
}
func (t *TestTailer1) GetInfo() *status.InfoRegistry {
	return t.info
}

type TestTailer2 struct {
	id   string
	info *status.InfoRegistry
}

func NewTestTailer2(id string) *TestTailer2 {
	return &TestTailer2{
		id:   id,
		info: status.NewInfoRegistry(),
	}
}

//nolint:revive // TODO(AML) Fix revive linter
func (t *TestTailer2) GetId() string {
	return t.id
}

//nolint:revive // TODO(AML) Fix revive linter
func (t *TestTailer2) GetType() string {
	return "test"
}

//nolint:revive // TODO(AML) Fix revive linter
func (t *TestTailer2) GetInfo() *status.InfoRegistry {
	return t.info
}

func TestCollectAllTailers(t *testing.T) {

	container1 := NewTailerContainer[*TestTailer1]()
	container1.Add(NewTestTailer1("1a"))
	t1b := NewTestTailer1("1b")
	container1.Add(t1b)

	container2 := NewTailerContainer[*TestTailer2]()
	container2.Add(NewTestTailer2("2a"))
	container2.Add(NewTestTailer2("2b"))

	tracker := NewTailerTracker()
	tracker.Add(container1)
	tracker.Add(container2)

	tailers := tracker.All()
	assert.Equal(t, 4, len(tailers))

	results := make(map[string]bool)
	for _, t := range tailers {
		results[t.GetId()] = true
	}

	for _, k := range []string{"1a", "1b", "2a", "2b"} {
		assert.True(t, results[k])
	}

	container1.Remove(t1b)

	results = make(map[string]bool)
	for _, t := range tailers {
		results[t.GetId()] = true
	}

	for _, k := range []string{"1a", "2a", "2b"} {
		assert.True(t, results[k])
	}

}
