// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tagger

// ListResponse holds all info in store
type ListResponse struct {
	Entities map[string]ListEntity `json:"entities"`
}

// ListEntity holds the tagging info about an entity
type ListEntity struct {
	Sources []string `json:"sources"`
	Tags    []string `json:"tags"`
}
