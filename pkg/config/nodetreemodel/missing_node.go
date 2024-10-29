// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// missingLeafImpl is a none-object representing when a child node is missing
type missingLeafImpl struct{}

var _ Node = (*missingLeafImpl)(nil)

var missingLeaf = &missingLeafImpl{}

func (m *missingLeafImpl) GetChild(string) (Node, error) {
	return nil, fmt.Errorf("GetChild(): missing")
}

func (m *missingLeafImpl) GetAny() (any, error) {
	return nil, fmt.Errorf("GetAny(): missing")
}

func (m *missingLeafImpl) GetBool() (bool, error) {
	return false, fmt.Errorf("GetBool(): missing")
}

func (m *missingLeafImpl) GetInt() (int, error) {
	return 0, fmt.Errorf("GetInt(): missing")
}

func (m *missingLeafImpl) GetFloat() (float64, error) {
	return 0.0, fmt.Errorf("GetFloat(): missing")
}

func (m *missingLeafImpl) GetString() (string, error) {
	return "", fmt.Errorf("GetString(): missing")
}

func (m *missingLeafImpl) GetTime() (time.Time, error) {
	return time.Time{}, fmt.Errorf("GetTime(): missing")
}

func (m *missingLeafImpl) GetDuration() (time.Duration, error) {
	return time.Duration(0), fmt.Errorf("GetDuration(): missing")
}

func (m *missingLeafImpl) SetWithSource(interface{}, model.Source) error {
	return fmt.Errorf("SetWithSource(): missing")
}

func (m *missingLeafImpl) Source() model.Source {
	return model.SourceUnknown
}

func (m *missingLeafImpl) Clone() Node {
	return m
}

func (m *missingLeafImpl) SourceGreaterOrEqual(model.Source) bool {
	return false
}
