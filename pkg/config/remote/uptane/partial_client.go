// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

// PartialClient is a partial uptane client
type PartialClient struct {
}

// NewPartialClient creates a new partial uptane client
func NewPartialClient() *PartialClient {
	return &PartialClient{}
}

// Verify is not implemented
func (c *PartialClient) Verify(response *pbgo.ConfigResponse) error {
	return fmt.Errorf("not implemented")
}
