// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import "github.com/uptrace/bun"

type DummyTable struct {
	bun.BaseModel `bun:"table:dummy,alias:d"`

	ID  int64 `bun:",pk,autoincrement"`
	Foo string
}
