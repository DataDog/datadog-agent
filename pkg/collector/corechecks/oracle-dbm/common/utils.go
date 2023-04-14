// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"hash/fnv"
	"strconv"

	"github.com/twmb/murmur3"
)

func GetQuerySignature(statement string) string {
	h := fnv.New64a()
	h.Write([]byte(statement))
	return strconv.FormatUint(murmur3.Sum64([]byte(statement)), 16)
}

type ObfuscatedStatement struct {
	Statement      string
	QuerySignature string
	Commands       []string
	Tables         []string
	Comments       []string
}
