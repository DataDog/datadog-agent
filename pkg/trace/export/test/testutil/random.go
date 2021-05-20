// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"bytes"
	"math/rand"
	"strconv"
)

// RandomSizedBytes creates a random byte slice with the specified size.
func RandomSizedBytes(size int) []byte {
	buffer := bytes.Buffer{}

	for i := 0; i < size; i++ {
		buffer.WriteByte(byte(rand.Int()))
	}

	return buffer.Bytes()
}

// RandomStringMap creates a random map with string keys and values.
func RandomStringMap() map[string]string {
	length := rand.Intn(32)

	m := map[string]string{}

	for i := 0; i < length; i++ {
		m[strconv.Itoa(rand.Int())] = strconv.Itoa(rand.Int())
	}

	return m
}
