// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/DataDog/datadog-agent/test/fakeintake/server/serverstore"
)

// StopAndExport stops admission, joins accepted handlers and retention cleanup,
// then writes retained original payloads before closing the store. The caller
// owns the writer and its close result. Metadata retains parsed request values,
// not original HTTP wire framing. Previously expired history is not recovered.
// Only one Stop or StopAndExport call can stop the server; later calls return an
// error, including after an export failure. A failed export cannot be retried.
func (fi *Server) StopAndExport(output io.Writer) error {
	if output == nil {
		return errors.New("payload export writer is required")
	}
	return fi.stop(output)
}

func (fi *Server) exportPayloads(output io.Writer) error {
	if _, err := io.WriteString(output, `{"payloads":{`); err != nil {
		return err
	}
	routes := make([]string, 0)
	for route := range fi.store.GetRouteStats() {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	encoder := json.NewEncoder(output)
	for index, route := range routes {
		if index != 0 {
			if _, err := io.WriteString(output, ","); err != nil {
				return err
			}
		}
		if err := encoder.Encode(route); err != nil {
			return err
		}
		if _, err := io.WriteString(output, ":["); err != nil {
			return err
		}
		cursor, first := "", true
		for {
			page, err := fi.store.GetRawPayloadPage(route, cursor, serverstore.MaxPageSize)
			if err != nil {
				return err
			}
			for _, payload := range page.Payloads {
				if !first {
					if _, err := io.WriteString(output, ","); err != nil {
						return err
					}
				}
				if err := encoder.Encode(payload); err != nil {
					return err
				}
				first = false
			}
			if !page.HasMore {
				break
			}
			cursor = page.NextCursor
		}
		if _, err := io.WriteString(output, "]"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(output, "}}\n")
	return err
}
