// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

func (c *Check) connectionTest() error {
	var n float64
	err := c.db.Get(&n, "SELECT 1 FROM dual")
	return err
}
