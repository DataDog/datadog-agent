// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package software

// SoftwareInventoryWireEntry is the wire format for software inventory entries
// sent between the Agent and the System Probe. It includes all Entry fields
// with explicit JSON tags so internal-only fields (e.g. install_path) are
// preserved over the wire. The backend payload uses Entry and omits
// internal fields via json:"-"; this type is not sent to the backend.
// Some fields (e.g. InstallPath, InstallPaths, PkgID, InstallSource) are
// primarily used on macOS but present for all platforms for future use.
type SoftwareInventoryWireEntry struct {
	Source       string `json:"software_type"`
	DisplayName  string `json:"name"`
	Version      string `json:"version"`
	InstallDate  string `json:"deployment_time,omitempty"`
	UserSID      string `json:"user,omitempty"`
	Is64Bit      bool   `json:"is_64_bit"`
	Publisher    string `json:"publisher"`
	Status       string `json:"deployment_status"`
	BrokenReason string `json:"broken_reason,omitempty"`
	ProductCode  string `json:"product_code"`
	// Internal / macOS-primary fields (preserved on wire; not sent to backend):
	InstallSource string   `json:"install_source,omitempty"`
	PkgID         string   `json:"pkg_id,omitempty"`
	InstallPath   string   `json:"install_path,omitempty"` // Internal for Windows and macOS
	InstallPaths  []string `json:"install_paths,omitempty"`
}

// EntryToWire converts an Entry to the wire format for Agent–System Probe communication.
func EntryToWire(e *Entry) SoftwareInventoryWireEntry {
	if e == nil {
		return SoftwareInventoryWireEntry{}
	}
	w := SoftwareInventoryWireEntry{
		Source:        e.Source,
		DisplayName:   e.DisplayName,
		Version:       e.Version,
		InstallDate:   e.InstallDate,
		UserSID:       e.UserSID,
		Is64Bit:       e.Is64Bit,
		Publisher:     e.Publisher,
		Status:        e.Status,
		BrokenReason:  e.BrokenReason,
		ProductCode:   e.ProductCode,
		InstallSource: e.InstallSource,
		PkgID:         e.PkgID,
		InstallPath:   e.InstallPath,
	}
	if len(e.InstallPaths) > 0 {
		w.InstallPaths = make([]string, len(e.InstallPaths))
		copy(w.InstallPaths, e.InstallPaths)
	}
	return w
}

// WireToEntry converts a wire-format entry back to an Entry.
func WireToEntry(w *SoftwareInventoryWireEntry) Entry {
	if w == nil {
		return Entry{}
	}
	e := Entry{
		Source:        w.Source,
		DisplayName:   w.DisplayName,
		Version:       w.Version,
		InstallDate:   w.InstallDate,
		UserSID:       w.UserSID,
		Is64Bit:       w.Is64Bit,
		Publisher:     w.Publisher,
		Status:        w.Status,
		BrokenReason:  w.BrokenReason,
		ProductCode:   w.ProductCode,
		InstallSource: w.InstallSource,
		PkgID:         w.PkgID,
		InstallPath:   w.InstallPath,
	}
	if len(w.InstallPaths) > 0 {
		e.InstallPaths = make([]string, len(w.InstallPaths))
		copy(e.InstallPaths, w.InstallPaths)
	}
	return e
}
