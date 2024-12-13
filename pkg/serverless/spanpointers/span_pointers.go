// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package spanpointers provides a helper functions for span pointers
package spanpointers

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"strings"
)

const (
	s3PointerKind = "aws.s3.object"
)

// SpanPointer is a struct that stores a hash and span kind to uniquely
// identify a S3 or DynamoDB operation.
type SpanPointer struct {
	Hash string
	Kind string
}

func generateSpanPointerHash(components []string) string {
	dataToHash := strings.Join(components, "|")
	sum := sha256.Sum256([]byte(dataToHash))
	return hex.EncodeToString(sum[:])[:32]
}

// GetSpanPointersFromS3Event calculates span pointer attributes to uniquely identify
// S3 event records. These attributes will later be used to create the _dd.span_links JSON object.
func GetSpanPointersFromS3Event(event events.S3Event) []SpanPointer {
	var pointers []SpanPointer
	for _, record := range event.Records {
		bucketName := record.S3.Bucket.Name
		key := record.S3.Object.Key
		eTag := strings.Trim(record.S3.Object.ETag, "\"")

		hash := generateSpanPointerHash([]string{bucketName, key, eTag})

		spanPointer := SpanPointer{
			Hash: hash,
			Kind: s3PointerKind,
		}
		pointers = append(pointers, spanPointer)
	}
	return pointers
}
