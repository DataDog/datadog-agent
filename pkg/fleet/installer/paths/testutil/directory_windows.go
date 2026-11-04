// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"os"
	"testing"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func temporaryDir(t testing.TB, trusted bool) string {
	t.Helper()
	dir := t.TempDir()
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		t.Fatalf("cannot determine temporary directory owner: %v", err)
	}
	// Inspect ownership independently of the production trust gate. Unelevated runners
	// already create untrusted roots; Administrators/SYSTEM/container administrators do not.
	isTrusted := owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) ||
		owner.IsWellKnown(windows.WinLocalSystemSid) || owner.String() == "S-1-5-93-2-1"
	if isTrusted == trusted {
		return dir
	}
	sidType := windows.WELL_KNOWN_SID_TYPE(windows.WinBuiltinGuestsSid)
	if trusted {
		sidType = windows.WinBuiltinAdministratorsSid
	}
	sid, err := windows.CreateWellKnownSid(sidType)
	if err != nil {
		t.Fatal(err)
	}
	privilegeEnabled := false
	err = winio.RunWithPrivileges([]string{"SeRestorePrivilege"}, func() error {
		privilegeEnabled = true
		return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, sid, nil, nil, nil)
	})
	if err != nil {
		// Never skip CI or hide a failure after privileges were acquired.
		if !privilegeEnabled && os.Getenv("CI") == "" && os.Getenv("CI_JOB_ID") == "" {
			t.Skipf("directory ownership fixture requires SeRestorePrivilege: %v", err)
		}
		t.Fatal(err)
	}
	return dir
}
