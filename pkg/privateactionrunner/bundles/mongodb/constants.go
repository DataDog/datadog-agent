// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_mongodb

const (
	maxOutputSize  = 15 * 1024 * 1024 // 15 MB cap on response size
	maxFilterDepth = 100              // Maximum nesting depth for filter validation
)

var allowedOperators = []string{
	"$eq", "$ne", "$gt", "$gte",
	"$lt", "$lte", "$in", "$nin",
	"$exists", "$type", "$and", "$or", "$nor",
}

const filterHelpMessage = `A valid filter in MongoDB must adhere to the following guidelines:
1. Operators: 
   - Operators must begin with a $ symbol and be one of the following: 
     - Comparison Operators: $eq, $ne, $gt, $gte, $lt, $lte
     - Logical Operators: $and, $or, $nor
     - Element Operators: $exists, $type
     - Array Operators: $in, $nin

2. Field Values:
   - Field values can be one of the following types:
     - Primitive Types: string, int, float64, bool
     - Nested Filters: A map[string]any where the value is another filter.
     
3. Logical Operators:
   - Logical operators ($and, $or, $nor) expect an array of filters as their value.
   - Each filter in the array must itself be a valid filter object.

4. Examples:
   - {"age": {"$gt": 30}} - Valid filter with a comparison operator.
   - {"$and": [{"age": {"$gt": 30}}, {"status": "active"}]} - Valid filter using a logical operator.

Ensure your filters conform to these guidelines to avoid validation errors.`
