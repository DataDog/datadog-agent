// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"context"
	"fmt"

	credssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/credentials"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type InsertManyAction struct{}

func NewInsertManyHandler() types.Action {
	return &InsertManyAction{}
}

type InsertManyInputs struct {
	Collection string                     `json:"collection,omitempty"`
	Documents  []interface{}              `json:"documents,omitempty"`
	Options    *options.InsertManyOptions `json:"options,omitempty"`
}

type InsertManyOutputs struct {
	InsertedIDs []interface{} `json:"insertedIds"`
}

func (ima InsertManyAction) Run(ctx context.Context, task *types.Task, credential interface{}) (interface{}, error) {
	inputs, err := types.ExtractInputs[InsertManyInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to extract inputs: %w", err)
	}

	credentialTokens, err := credssupport.ToTokensMap(credential)
	if err != nil {
		return nil, fmt.Errorf("failed to convert credentials: %w", err)
	}

	clientOptions, cs, err := createMongoClientOptions(ctx, credentialTokens)
	if err != nil {
		return nil, err
	}

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Error("Error disconnecting from MongoDB %w", err)
		}
	}()

	db := client.Database(cs.Database)
	collection := db.Collection(inputs.Collection)

	result, err := collection.InsertMany(ctx, inputs.Documents, inputs.Options)
	if err != nil {
		return nil, fmt.Errorf("Failed to insert documents: %w", err)
	}

	return &InsertManyOutputs{
		InsertedIDs: result.InsertedIDs,
	}, nil
}
