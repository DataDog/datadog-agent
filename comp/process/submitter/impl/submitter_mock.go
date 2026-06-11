// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package submitterimpl

import (
	"testing"

	"github.com/stretchr/testify/mock"

	submitterComp "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	"github.com/DataDog/datadog-agent/pkg/process/runner/mocks"
)

// NewMock creates a new mock submitter component for testing.
func NewMock(t testing.TB) submitterComp.Component {
	s := mocks.NewSubmitter(t)
	s.On("Start").Maybe().Return(nil)
	s.On("Stop").Maybe()
	s.On("Submit", mock.Anything, mock.Anything, mock.Anything).Maybe()
	return s
}
