// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import "github.com/apache/arrow-go/v18/arrow/array"

// readStringList reads a string list from an Arrow List column at the given row.
func readStringList(col *array.List, row int) []string {
	if col.IsNull(row) {
		return nil
	}
	start, end := col.ValueOffsets(row)
	values := col.ListValues().(*array.String)
	result := make([]string, end-start)
	for j := start; j < end; j++ {
		result[j-start] = values.Value(int(j))
	}
	return result
}
