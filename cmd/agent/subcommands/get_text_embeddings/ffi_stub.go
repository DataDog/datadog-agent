// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo || !(linux || darwin)

package get_text_embeddings

import "fmt"

func printWithRust(_ string) error {
	return fmt.Errorf("get-text-embeddings requires cgo on linux or darwin")
}
