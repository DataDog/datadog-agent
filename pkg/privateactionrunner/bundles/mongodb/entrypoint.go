// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type MongoDB struct {
}

func NewMongoDB() types.Bundle {
	return &MongoDB{}
}

func (m *MongoDB) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "find":
		return m.RunFind(ctx, task, credential)
	case "insertOne":
		return m.RunInsertOne(ctx, task, credential)
	case "updateOne":
		return m.RunUpdateOne(ctx, task, credential)
	case "replaceOne":
		return m.RunReplaceOne(ctx, task, credential)
	case "countDocuments":
		return m.RunCountDocuments(ctx, task, credential)
	case "updateMany":
		return m.RunUpdateMany(ctx, task, credential)
	case "createIndex":
		return m.RunCreateIndex(ctx, task, credential)
	case "findAndModify":
		return m.RunFindAndModify(ctx, task, credential)
	case "insertMany":
		return m.RunInsertMany(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (m *MongoDB) GetAction(actionName string) types.Action {
	return m
}
