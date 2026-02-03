// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo || !(linux || darwin)

package deepinference

import "fmt"

func Init() error {
	return fmt.Errorf("deepinference requires cgo on linux or darwin")
}

func GetEmbeddingsSize() (int, error) {
	return 0, fmt.Errorf("deepinference requires cgo on linux or darwin")
}

func GetEmbeddings(text string) ([]float32, error) {
	return nil, fmt.Errorf("deepinference requires cgo on linux or darwin")
}

func Benchmark() error {
	return fmt.Errorf("deepinference requires cgo on linux or darwin")
}
