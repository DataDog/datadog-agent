// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import "github.com/DataDog/datadog-agent/pkg/proto/pbgo"

type PartialClient struct {
}

func NewPartialClient() *PartialClient {
	return &PartialClient{}
}

func (c *PartialClient) Verify(response *pbgo.ConfigResponse) error {
	return nil
}
