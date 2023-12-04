// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagent

type inventoryagentMock struct{}

func newMock() Component {
	return &inventoryagentMock{}
}

func (m *inventoryagentMock) Set(string, interface{}) {}

func (m *inventoryagentMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (m *inventoryagentMock) Get() map[string]interface{} {
	return nil
}

func (m *inventoryagentMock) Refresh() {}
