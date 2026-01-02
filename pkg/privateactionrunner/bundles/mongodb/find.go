// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type FindAction struct{}

func NewFindHandler() types.Action {
	return &FindAction{}
}

type FindInputs struct {
	Collection string         `json:"collection,omitempty"`
	Filter     map[string]any `json:"filter,omitempty"`
}

type FindOutputs struct {
	Results []map[string]interface{} `json:"results"`
}

func (fa FindAction) Run(ctx context.Context, task *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error) {
	inputs, err := types.ExtractInputs[FindInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to extract inputs: %w", err)
	}

	if err := ValidateFilter(inputs.Filter); err != nil {
		return nil, err
	}

	credentialTokens := credential.AsTokenMap()
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
			log.FromContext(ctx).Error("Error disconnecting from MongoDB", log.ErrorField(err))
		}
	}()

	db := client.Database(cs.Database)
	collection := db.Collection(inputs.Collection)

	cur, err := collection.Find(ctx, inputs.Filter)
	if err != nil {
		return nil, fmt.Errorf("error finding documents: %w", err)
	}
	defer func() {
		if err := cur.Close(ctx); err != nil {
			log.FromContext(ctx).Error("Error closing cursor", log.ErrorField(err))
		}
	}()

	var results []map[string]interface{}
	currentSize := 0

	for cur.Next(ctx) {
		var document map[string]interface{}
		if err := cur.Decode(&document); err != nil {
			return nil, fmt.Errorf("error decoding result: %w", err)
		}

		estimatedSize, err := json.Marshal(document)
		if err != nil {
			return nil, fmt.Errorf("error estimating document size: %w", err)
		}

		if currentSize+len(estimatedSize) > maxOutputSize {
			break
		}

		results = append(results, document)
		currentSize += len(estimatedSize)
	}

	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return &FindOutputs{
		Results: results,
	}, nil
}
