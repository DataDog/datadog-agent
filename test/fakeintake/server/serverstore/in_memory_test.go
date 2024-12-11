// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestInMemoryStore(t *testing.T) {
	suite.Run(t, &StoreTestSuite{
		StoreConstructor: func() Store {
			return newInMemoryStore()
		},
	})
}
