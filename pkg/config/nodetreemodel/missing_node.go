// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// missingLeafImpl is a none-object representing when a child node is missing
type missingLeafImpl struct{}

var _ Node = (*missingLeafImpl)(nil)

var missingLeaf = &missingLeafImpl{}

func (m *missingLeafImpl) GetChild(string) (Node, error) {
	return nil, errors.New("GetChild(): missing")
}

func (m *missingLeafImpl) Get() interface{} {
	return nil
}

func (m *missingLeafImpl) ReplaceValue(interface{}) error {
	return errors.New("Replacevalue(): missing")
}

func (m *missingLeafImpl) SetWithSource(interface{}, model.Source) error {
	return errors.New("SetWithSource(): missing")
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
