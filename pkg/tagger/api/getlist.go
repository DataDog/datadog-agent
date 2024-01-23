// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api implements the Tagger API.
package api

import (
	"io"
)

// GetTaggerList display in a human readable format the Tagger entities into the io.Write w.
func GetTaggerList(w io.Writer, url string) error {
	panic("not called")
}

// printTaggerEntities use to print Tagger entities into an io.Writer
func printTaggerEntities(w io.Writer, tr *TaggerListResponse) {
	panic("not called")
}
