// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

// Parsing for the two Group Policy Object list properties.
//
// Both GPOInfoList (event 5312) and ApplicableGPOList (event 4016) are declared
// win:UnicodeString rather than a TDH array, so the TDH layer hands them over
// as one opaque string and the structure has to be recovered here.
//
// GPOInfoList is an XML fragment with no root element: a bare sequence of
// sibling <GPO ID="{GUID}"> entries. xml.Unmarshal stops after the first
// top-level element, so a token loop is used instead. That also means a rooted
// variant would parse correctly, which matters because no first-party capture
// of either property exists to confirm the exact runtime shape - the field
// types come from the provider manifest and the structure from third-party
// captures. Every path here degrades to fewer results rather than guessing.

// errGPOListTooLarge reports a list rejected before parsing.
var errGPOListTooLarge = errors.New("GPO list exceeds the maximum parsable size")

// bracedGUIDPattern matches a GUID in braced form anywhere in a string.
var bracedGUIDPattern = regexp.MustCompile(
	`\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}`)

// gpoXML is one <GPO> element of a GPO list fragment.
type gpoXML struct {
	ID      string `xml:"ID,attr"`
	Name    string `xml:"Name"`
	Version string `xml:"Version"`
	SOM     string `xml:"SOM"`
}

// parseGPOList extracts GPO metadata from a GPOInfoList or ApplicableGPOList
// property value.
//
// It returns everything it managed to decode alongside any error, so a
// truncated or partly malformed list still contributes the entries that parsed.
// Entries with no resolvable GUID are dropped: the GUID is the identity, and a
// display name is neither unique nor stable enough to stand in for it.
func parseGPOList(raw string) ([]GPO, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > maxGPOListBytes {
		return nil, errGPOListTooLarge
	}

	var gpos []GPO
	decoder := xml.NewDecoder(strings.NewReader(raw))

	for len(gpos) < maxGPOsPerList {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) || token == nil {
			break
		}
		if err != nil {
			return gpos, err
		}

		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "GPO" {
			// Not a GPO element. Keep walking rather than skipping the subtree:
			// the fragment has no root, so a wrapper element may enclose the
			// entries we want.
			continue
		}

		var entry gpoXML
		if err := decoder.DecodeElement(&entry, &start); err != nil {
			return gpos, err
		}
		if gpo, ok := gpoFromXML(entry, start); ok {
			gpos = append(gpos, gpo)
		}
	}
	return gpos, nil
}

// gpoFromXML normalizes one decoded element, returning false when it carries no
// usable identity.
func gpoFromXML(entry gpoXML, start xml.StartElement) (GPO, bool) {
	id := entry.ID
	if id == "" {
		// The attribute name's casing is not confirmed against a live capture,
		// so fall back to a case-insensitive scan before giving up.
		for _, attr := range start.Attr {
			if strings.EqualFold(attr.Name.Local, "id") {
				id = attr.Value
				break
			}
		}
	}

	_, normalized, ok := normalizeGUID(id)
	if !ok {
		return GPO{}, false
	}
	return GPO{
		ID:      normalized,
		Name:    truncateProviderText(entry.Name, maxGPONameBytes),
		SOM:     truncateProviderText(entry.SOM, maxGPOSOMBytes),
		Version: truncateProviderText(entry.Version, maxGPOVersion),
	}, true
}

// parseApplicableGPOList extracts the GPO references carried by a 4016 event.
//
// The runtime format of ApplicableGPOList has not been observed first-hand. The
// XML fragment parser is tried first; if it yields nothing, the value is
// scanned for braced GUIDs instead, which recovers the references regardless of
// how the provider delimits them. The second tier reports only IDs, so any
// metadata for those GPOs has to come from a 5312 event.
//
// Returns the normalized GUIDs, the metadata the first tier recovered (empty
// when the fallback was used), and whether parsing degraded.
func parseApplicableGPOList(raw string) (ids []string, gpos []GPO, degraded bool) {
	if raw == "" {
		return nil, nil, false
	}

	gpos, err := parseGPOList(raw)
	if len(gpos) > 0 {
		for _, g := range gpos {
			ids = appendUniqueGPOID(ids, g.ID)
		}
		return ids, gpos, err != nil
	}
	if errors.Is(err, errGPOListTooLarge) {
		return nil, nil, true
	}

	for _, match := range bracedGUIDPattern.FindAllString(raw, -1) {
		if _, normalized, ok := normalizeGUID(match); ok {
			ids = appendUniqueGPOID(ids, normalized)
		}
	}
	return ids, nil, len(ids) == 0
}

// appendUniqueGPOID adds an ID unless it is empty, already present, or the
// per-invocation bound has been reached. The list is short enough that a linear
// scan beats maintaining a set.
func appendUniqueGPOID(ids []string, id string) []string {
	if id == "" || len(ids) >= maxApplicableGPOIDsPerCSE {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}
