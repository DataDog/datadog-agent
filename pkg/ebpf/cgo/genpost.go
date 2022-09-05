// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"go/format"
	"io/ioutil"
	"log"
	"os"
	"regexp"
)

func main() {
	b, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	removeAbsolutePathRegex := regexp.MustCompile(`(// cgo -godefs [^/]+) /.+/([^/]+)$`)
	b = removeAbsolutePathRegex.ReplaceAll(b, []byte("$1 $2"))

	// Convert [160]int8 to [160]byte in http_transaction_t members to simplify
	// conversion to string; see golang.org/issue/20753
	convertHttpTransactionRegex := regexp.MustCompile(`(Request_fragment)(\s+)\[(\d+)\]u?int8`)
	b = convertHttpTransactionRegex.ReplaceAll(b, []byte("$1$2[$3]byte"))

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
