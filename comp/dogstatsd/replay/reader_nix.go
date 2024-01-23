// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package replay

// getFileContent returns a slice of bytes with the contents of the file specified in the path.
// The mmap flag will try to MMap file so as to achieve reasonable performance with very large
// files while not loading the entire thing into memory.
func getFileContent(path string, mmap bool) ([]byte, error) {
	panic("not called")
}

func unmapFile(b []byte) error {
	panic("not called")
}
