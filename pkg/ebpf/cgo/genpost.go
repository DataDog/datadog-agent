// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"go/format"
	"io"
	"log"
	"os"
	"regexp"
	"runtime"
)

func main() {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	b = removeAbsolutePath(b, runtime.GOOS)

	// Convert [160]int8 to [160]byte in http_transaction_t members to simplify
	// conversion to string; see golang.org/issue/20753
	convertHTTPTransactionRegex := regexp.MustCompile(`(Request_fragment)(\s+)\[(\d+)\]u?int8`)
	b = convertHTTPTransactionRegex.ReplaceAll(b, []byte("$1$2[$3]byte"))

	// Convert [120]int8 to [120]byte in lib_path_t members to simplify
	// conversion to string; see golang.org/issue/20753
	convertLibraryRegex := regexp.MustCompile(`(Buf)(\s+)\[(\d+)\]u?int8`)
	b = convertLibraryRegex.ReplaceAll(b, []byte("$1$2[$3]byte"))

	b, err = format.Source(b)
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(b)
}

// removeAbsolutePath removes the absolute file path that is automatically output by cgo -godefs
// and replaces it with only the filename
func removeAbsolutePath(b []byte, platform string) []byte {
	var removeAbsolutePathRegex *regexp.Regexp
	switch platform {
	case "linux":
		removeAbsolutePathRegex = regexp.MustCompile(`(// cgo -godefs .+) /.+/([^/]+)$`)
	case "windows":
		removeAbsolutePathRegex = regexp.MustCompile(`(// cgo.exe -godefs .+) .:\\.+\\([^\\]+)$`)
	default:
		log.Fatal("unsupported platform")
	}

	return removeAbsolutePathRegex.ReplaceAll(b, []byte("$1 $2"))
}
