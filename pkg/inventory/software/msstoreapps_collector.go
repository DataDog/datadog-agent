// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	msstoreapps "github.com/DataDog/datadog-agent/pkg/util/winutil/datadoginterop"
)

// msStoreAppsCollector collects Windows Store apps using libdatadog-interop.dll.
type msStoreAppsCollector struct{}

func (c *msStoreAppsCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning

	store, err := msstoreapps.GetStore()
	if err != nil {
		return nil, warnings, err
	}

	if store.Count == 0 {
		return entries, warnings, nil
	}

	raw := unsafe.Slice(store.Entries, int(store.Count))
	for _, e := range raw {
		version := fmt.Sprintf("%d.%d.%d.%d", e.VersionMajor, e.VersionMinor, e.VersionBuild, e.VersionRevision)

		var installDate string
		if e.InstallDate > 0 {
			t := time.Unix(0, e.InstallDate*int64(time.Millisecond))
			installDate = t.UTC().Format(time.RFC3339)
		}

		entries = append(entries, &Entry{
			DisplayName: windows.UTF16PtrToString(e.DisplayName),
			Version:     version,
			InstallDate: installDate,
			Source:      softwareTypeMSStore,
			UserSID:     "",
			Is64Bit:     e.Is64Bit == 1,
			Publisher:   windows.UTF16PtrToString(e.Publisher),
			Status:      "installed",
			ProductCode: windows.UTF16PtrToString(e.ProductCode),
		})
	}

	err = msstoreapps.FreeStore(store)
	if err != nil {
		return nil, warnings, err
	}

	return entries, warnings, nil
}
