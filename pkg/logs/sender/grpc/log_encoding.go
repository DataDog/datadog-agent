// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

const emptyDictIndex uint64 = 1

func logDictIndex(id uint64) uint64 {
	if id == 0 {
		return emptyDictIndex
	}
	return id
}

func isEmptyDictIndex(id uint64) bool {
	return id == 0 || id == emptyDictIndex
}
