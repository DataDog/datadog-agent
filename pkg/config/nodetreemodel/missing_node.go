// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// missingLeafImpl is a none-object representing when a child node is missing
type missingLeafImpl struct{}

var _ Node = (*missingLeafImpl)(nil)

var missingLeaf = &missingLeafImpl{}

func (m *missingLeafImpl) GetChild(string) (Node, error) {
	return nil, fmt.Errorf("GetChild(): missing")
}

func (m *missingLeafImpl) Get() interface{} {
	return nil
}

func (m *missingLeafImpl) ReplaceValue(interface{}) error {
	return fmt.Errorf("Replacevalue(): missing")
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

func (m *missingLeafImpl) SourceGreaterThan(model.Source) bool {
	return false
}
