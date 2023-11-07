// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"strings"
)

// Event Keywords fields
// https://learn.microsoft.com/en-us/dotnet/api/system.diagnostics.eventing.reader.standardeventkeywords
const (
	failureAuditFlag uint64 = 0x10000000000000
	successAuditFlag uint64 = 0x20000000000000
)

type filterDefinition interface {
	Sources() []string
	Types() []string
	IDs() []int
}

func filterIsEmpty(f filterDefinition) bool {
	return f == nil || (len(f.Sources()) == 0 &&
		len(f.Types()) == 0 &&
		len(f.IDs()) == 0)
}

// queryFromFilter converts the filter definition from the config into a structured XML query
//
// Examples of XPath queries:
// - https://powershell.org/2019/08/a-better-way-to-search-events/
// - https://www.petri.com/query-xml-event-log-data-using-xpath-in-windows-server-2012-r2
// - https://blog.backslasher.net/filtering-windows-event-log-using-xpath.html
func queryFromFilter(f filterDefinition) (string, error) {
	if filterIsEmpty(f) {
		return "*", nil
	}
	sourcePart, err := genQueryPart(f.Sources(), formatSourcePart)
	if err != nil {
		return "", err
	}
	if len(sourcePart) > 0 {
		sourcePart = fmt.Sprintf("Provider[%s]", sourcePart)
	}
	typePart, err := genQueryPart(f.Types(), formatTypePart)
	if err != nil {
		return "", err
	}
	eventIDPart, err := genQueryPart(f.IDs(), formatEventIDPart)
	if err != nil {
		return "", err
	}
	systemValsQuery := logicJoinParts([]string{
		sourcePart,
		typePart,
		eventIDPart,
	}, "and")
	return fmt.Sprintf("*[System[%s]]", systemValsQuery), nil
}

func genQueryPart[T string | int](vals []T, formatVal func(T) (string, error)) (string, error) {
	var err error
	if len(vals) == 0 {
		return "", nil
	}
	parts := make([]string, len(vals))
	for i, val := range vals {
		parts[i], err = formatVal(val)
		if err != nil {
			return "", err
		}
	}
	return logicJoinParts(parts, "or"), nil
}

func formatSourcePart(source string) (string, error) {
	part := fmt.Sprintf("@Name=%s", xpathQuoteString(source))
	return part, nil
}

func formatEventIDPart(eventID int) (string, error) {
	part := fmt.Sprintf("EventID=%d", eventID)
	return part, nil
}

// formatTypePart adds filters for event levels and the Security event log audit types
//
// level values
// https://learn.microsoft.com/en-us/windows/win32/wes/eventmanifestschema-leveltype-complextype#remarks
func formatTypePart(t string) (string, error) {
	// lowercase for case insensitive match
	t = strings.ToLower(t)
	var part string
	switch t {
	case "critical":
		part = "Level=1"
	case "error":
		part = "Level=2"
	case "warning":
		part = "Level=3"
	case "information":
		// Match event viewer behavior
		part = "(Level=0 or Level=4)"
	// NOTE: query does not support `0x` syntax for integer values
	case "failure audit":
		part = fmt.Sprintf("band(Keywords,%d)", failureAuditFlag)
	case "success audit":
		part = fmt.Sprintf("band(Keywords,%d)", successAuditFlag)
	default:
		return "", fmt.Errorf("invalid event level: %s", t)
	}
	return part, nil
}

// Cannot find explicit documentation on quote syntax, but Event Viewer uses single quotes
func xpathQuoteString(s string) string {
	return fmt.Sprintf("'%s'", s)
}

func logicJoinParts(parts []string, op string) string {
	// remove empty parts
	newparts := make([]string, len(parts))
	i := 0
	for _, part := range parts {
		if len(part) > 0 {
			newparts[i] = part
			i++
		}
	}
	newparts = newparts[:i]
	if len(newparts) == 1 {
		return newparts[0]
	}
	return fmt.Sprintf("(%s)", strings.Join(newparts, fmt.Sprintf(" %s ", op)))
}
