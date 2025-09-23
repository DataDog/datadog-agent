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
