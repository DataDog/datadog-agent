// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type testSuite struct {
	suite.Suite
}

func (s *testSuite) TestError() {
	_, err := GetLeaderEngine()
	require.NotNil(s.T(), err)
}

func TestSuite(t *testing.T) {
	s := &testSuite{}
	suite.Run(t, s)
}
