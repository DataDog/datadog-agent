// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package json

import (
	"encoding/json"
	"fmt"
	"io"
)

// PrintJSON writes JSON output to the provided writer, optionally pretty-printed
func PrintJSON(w io.Writer, rawJSON any, prettyPrintJSON bool) error {
	var result []byte
	var err error

	// convert to bytes and indent
	if prettyPrintJSON {
		result, err = json.MarshalIndent(rawJSON, "", "  ")
	} else {
		result, err = json.Marshal(rawJSON)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(result))
	if err != nil {
		return err
	}

	return nil
}
