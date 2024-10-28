// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

// Package jsonquery interacts with jq queries
package jsonquery

import (
	"fmt"
	"time"

	"github.com/itchyny/gojq"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheTTL      = 6 * time.Hour
	jqCachePrefix = "jq-"
)

// Parse returns an (eventually cached) Code object to run json query
func Parse(q string) (*gojq.Code, error) {
	if code, found := cache.Cache.Get(jqCachePrefix + q); found {
		return code.(*gojq.Code), nil
	}

	query, err := gojq.Parse(q)
	if err != nil {
		return nil, err
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return nil, err
	}

	if err := cache.Cache.Add(jqCachePrefix+q, code, cacheTTL); err != nil {
		log.Errorf("Unable to store item in cache: %v", err)
	}
	return code, nil
}

// RunSingleOutput runs a JQ query against `object` and returns the string value
// (assuming there is a single output)
func RunSingleOutput(q string, object interface{}) (string, bool, error) {
	code, err := Parse(q)
	if err != nil {
		return "", false, err
	}

	if value, ok := code.Run(object).Next(); ok {
		if err, ok := value.(error); ok {
			return "", false, err
		}

		if value == nil {
			return "", false, nil
		}

		return fmt.Sprint(value), true, nil
	}

	return "", false, nil
}
