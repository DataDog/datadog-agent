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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type UpdateManyInputs struct {
	Collection string         `json:"collection,omitempty"`
	Filter     map[string]any `json:"filter,omitempty"`
	Update     map[string]any `json:"update,omitempty"`
}

type UpdateManyOutputs struct {
	MatchedCount  int64 `json:"matchedCount"`
	ModifiedCount int64 `json:"modifiedCount"`
	UpsertedID    any   `json:"upsertedId,omitempty"`
}

func (m *MongoDB) RunUpdateMany(ctx context.Context, task *types.Task, credential interface{}) (interface{}, error) {
	inputs, err := types.ExtractInputs[UpdateManyInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to extract inputs: %w", err)
	}

	if err := ValidateFilter(inputs.Filter); err != nil {
		return nil, err
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

	result, err := collection.UpdateMany(ctx, inputs.Filter, inputs.Update)
	if err != nil {
		return nil, fmt.Errorf("Failed to update documents: %w", err)
	}

	var upsertedID string
	if result.UpsertedID != nil {
		objectID, ok := result.UpsertedID.(primitive.ObjectID)
		if !ok {
			return nil, fmt.Errorf("Failed to retrieve upserted ID")
		}
		upsertedID = objectID.Hex()
	}

	return &UpdateManyOutputs{
		MatchedCount:  result.MatchedCount,
		ModifiedCount: result.ModifiedCount,
		UpsertedID:    upsertedID,
	}, nil
}
