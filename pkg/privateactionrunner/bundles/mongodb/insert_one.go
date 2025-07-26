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

type InsertOneAction struct{}

func NewInsertOneHandler() types.Action {
	return &InsertOneAction{}
}

type InsertOneInputs struct {
	Collection string         `json:"collection,omitempty"`
	Document   map[string]any `json:"document,omitempty"`
}

type InsertOneOutputs struct {
	InsertedID string `json:"insertedId"`
}

func (ioa InsertOneAction) Run(ctx context.Context, task *types.Task, credential interface{}) (interface{}, error) {
	inputs, err := types.ExtractInputs[InsertOneInputs](task)
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

	result, err := collection.InsertOne(ctx, inputs.Document)
	if err != nil {
		return nil, fmt.Errorf("failed to insert document: %w", err)
	}

	objectID, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		return nil, fmt.Errorf("inserted ID is not a primitive.ObjectID")
	}
	insertedID := objectID.Hex()

	return &InsertOneOutputs{
		InsertedID: insertedID,
	}, nil
}
