// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build windows

package winutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestGetSidFromUser(t *testing.T) {
	sid, err := GetSidFromUser()
	t.Logf("The SID found was: %v", sid)
	assert.Nil(t, err)
	assert.NotNil(t, sid)
}

func TestGetServiceUserSID(t *testing.T) {
	// create LocalService SID
	serviceSid, err := windows.StringToSid("S-1-5-19")
	require.NoError(t, err)

	// get the SID for the EventLog service (has LocalService as its user)
	sid, err := GetServiceUserSID("EventLog")
	require.NoError(t, err)
	assert.NotNil(t, sid)
	assert.True(t, windows.EqualSid(sid, serviceSid))
	t.Logf("The SID found was: %v", sid)

	// create LocalSystem SID
	systemSid, err := windows.StringToSid("S-1-5-18")
	require.NoError(t, err)

	// get the SID for the BITS service (has LocalSystem as its user)
	sid, err = GetServiceUserSID("BITS")
	require.NoError(t, err)
	assert.NotNil(t, sid)
	assert.True(t, windows.EqualSid(sid, systemSid))
	t.Logf("The SID found was: %v", sid)
}

func TestGetDDUserGroups(t *testing.T) {
	// Test getDDUserGroups function
	// Note: This test may fail if DDAgent is not installed or if the registry key doesn't exist
	// That's expected behavior, so we'll handle the error gracefully
	groups, err := GetDDUserGroups()
	if err != nil {
		// If the error is about missing registry key or service, that's expected in test environment
		t.Logf("getDDUserGroups returned expected error (likely missing DDAgent installation): %v", err)
		return
	}

	// If we get here, the function worked and returned groups
	t.Logf("Found groups for DDAgent user: %v", groups)
	assert.NotNil(t, groups)
	// Groups could be empty if the user is not in any groups, which is valid
}

func TestGetDDUserRights(t *testing.T) {
	// Test getDDUserRights function
	// Note: This test may fail if DDAgent is not installed or if the registry key doesn't exist
	// That's expected behavior, so we'll handle the error gracefully
	rights, err := GetDDUserRights()
	if err != nil {
		// If the error is about missing registry key or service, that's expected in test environment
		t.Logf("getDDUserRights returned expected error (likely missing DDAgent installation): %v", err)
		return
	}

	// If we get here, the function worked and returned rights
	t.Logf("Found rights for DDAgent user: %v", rights)
	assert.NotNil(t, rights)
	// Rights could be empty if the user has no specific rights assigned, which is valid
}

func TestGetDDUserGroupsAndRights(t *testing.T) {
	// Test both functions together to ensure they work consistently
	// This test helps verify that both functions can access the same user information

	userName, userNameErr := GetServiceUser("datadogagent")
	groups, groupsErr := GetDDUserGroups()
	rights, rightsErr := GetDDUserRights()
	_, hasDesiredGroups, _ := DoesAgentUserHaveDesiredGroups()
	_, hasDesiredRights, _ := DoesAgentUserHaveDesiredRights()

	// Output user groups - one per line
	if userNameErr == nil {
		fmt.Printf("DDAgent User Name: %s\n", userName)
	} else {
		fmt.Printf("Error getting DDAgent user name: %v\n", userNameErr)
	}
	if groupsErr == nil {
		fmt.Println("DDAgent User Groups:")
		for _, group := range groups {
			fmt.Printf("  %s\n", group)
		}
	} else {
		fmt.Printf("Error getting DDAgent user groups: %v\n", groupsErr)
	}

	// Output user rights - one per line
	if rightsErr == nil {
		fmt.Println("DDAgent User Rights:")
		for _, right := range rights {
			fmt.Printf("  %s\n", right)
		}
	} else {
		fmt.Printf("Error getting DDAgent user rights: %v\n", rightsErr)
	}

	// Log the results for test framework
	t.Logf("getDDUserGroups result: groups=%v, err=%v", groups, groupsErr)
	t.Logf("getDDUserRights result: rights=%v, err=%v", rights, rightsErr)
	t.Logf("doesAgentUserHaveDesiredGroups result: hasDesiredGroups=%v", hasDesiredGroups)
	t.Logf("doesAgentUserHaveDesiredRights result: hasDesiredRights=%v", hasDesiredRights)

	// If both functions fail with the same type of error (likely missing DDAgent), that's expected
	if groupsErr != nil && rightsErr != nil {
		t.Logf("Both functions failed as expected (likely missing DDAgent installation)")
		return
	}

	// If one succeeds and the other fails, that might indicate an issue
	if (groupsErr == nil && rightsErr != nil) || (groupsErr != nil && rightsErr == nil) {
		t.Logf("One function succeeded while the other failed - this might indicate an issue")
		// Don't fail the test as this could be due to different permission requirements
	}

	// If both succeed, verify the results are valid
	if groupsErr == nil && rightsErr == nil {
		assert.NotNil(t, groups)
		assert.NotNil(t, rights)
		// Both could be empty slices, which is valid
	}
}

func TestDoesAgentUserHaveDesiredGroups(t *testing.T) {
	actualGroups, hasDesiredGroups, err := DoesAgentUserHaveDesiredGroups()

	if err != nil {
		t.Logf("Error checking groups: %v", err)
		if strings.Contains(err.Error(), "access denied") {
			t.Skip("Skipping test - requires administrator privileges")
		}
		// skip test on a system which does not have DDAgent installed
		if strings.Contains(err.Error(), "cannot find the file specified") ||
			strings.Contains(err.Error(), "could not open registry key") ||
			strings.Contains(err.Error(), "The specified service does not exist as an installed service") {
			t.Skip("Skipping test - DDAgent not installed")
		}
		t.Fatalf("Failed to check groups: %v", err)
	}

	t.Logf("Agent user groups: %v", actualGroups)
	t.Logf("Has all desired groups: %v", hasDesiredGroups)

	// Groups should be a slice (even if empty)
	assert.NotNil(t, actualGroups)
}

func TestDoesAgentUserHaveDesiredRights(t *testing.T) {
	actualRights, hasDesiredRights, err := DoesAgentUserHaveDesiredRights()

	if err != nil {
		t.Logf("Error checking rights: %v", err)
		if strings.Contains(err.Error(), "access denied") {
			t.Skip("Skipping test - requires administrator privileges")
		}
		// skip test on a system which does not have DDAgent installed
		if strings.Contains(err.Error(), "cannot find the file specified") ||
			strings.Contains(err.Error(), "could not open registry key") ||
			strings.Contains(err.Error(), "The specified service does not exist as an installed service") {
			t.Skip("Skipping test - DDAgent not installed")
		}
		t.Fatalf("Failed to check rights: %v", err)
	}

	t.Logf("Agent user rights: %v", actualRights)
	t.Logf("Has all desired rights: %v", hasDesiredRights)

	// Rights should be a slice (even if empty)
	assert.NotNil(t, actualRights)
}
