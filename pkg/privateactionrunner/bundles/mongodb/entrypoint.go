// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type MongoDB struct {
	actions map[string]types.Action
}

func NewMongoDB() types.Bundle {
	return &MongoDB{
		actions: map[string]types.Action{
			"find":           NewFindHandler(),
			"insertOne":      NewInsertOneHandler(),
			"updateOne":      NewUpdateOneHandler(),
			"replaceOne":     NewReplaceOneHandler(),
			"countDocuments": NewCountDocumentsHandler(),
			"updateMany":     NewUpdateManyHandler(),
			"createIndex":    NewCreateIndexHandler(),
			"findAndModify":  NewFindAndModifyHandler(),
			"insertMany":     NewInsertManyHandler(),
		},
	}
}

func (m MongoDB) GetAction(actionName string) types.Action {
	return m.actions[actionName]
}
