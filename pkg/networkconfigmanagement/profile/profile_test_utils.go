// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package profile

import (
	"embed"
	"fmt"
	"path"
	"regexp"
	"testing"
)

//go:embed fixtures/*
var fixturesFS embed.FS

// Fixture represents the data to pass in for the test and its expected output for profile definitions
type Fixture struct {
	Initial  []byte
	Expected []byte
}

func loadFixture(profileName ProfileName, command string) Fixture {
	initialPath := path.Join("fixtures", string(profileName), command, "initial.txt")
	initial, err := fixturesFS.ReadFile(initialPath)
	if err != nil {
		panic(fmt.Sprintf("could not load initial data fixture for profile: %s, command: %s, error: %s", profileName, command, err))
	}
	expectedPath := path.Join("fixtures", string(profileName), command, "expected.txt")
	expected, err := fixturesFS.ReadFile(expectedPath)
	if err != nil {
		panic(fmt.Sprintf("could not load expected data fixture for profile: %s, command:%s, error: %s", profileName, command, err))
	}
	return Fixture{
		Initial:  normalizeOutput(initial),
		Expected: normalizeOutput(expected),
	}
}

var exampleConfig = `
Building configuration...


Current configuration : 3144 bytes
!
! Last configuration change at 20:53:27 UTC Thu Aug 14 2025
!
version 15.9
service timestamps debug datetime msec
service timestamps log datetime msec
no service password-encryption
!
hostname qa-device
!
boot-start-marker
boot-end-marker
!
ip domain name lab.local
ip cef
no ipv6 cef
!
multilink bundle-name authenticated
!
!
!
!
username cisco privilege 15 secret 9 ooooooimasecretsdfsdjfdsnvZM5zmpPuLrKr9CZC3A1/jTwjHzA
!
redundancy
!
`

var expectedConfig = `
Building configuration...


Current configuration : 3144 bytes
!
! Last configuration change at 20:53:27 UTC Thu Aug 14 2025
!
version 15.9
service timestamps debug datetime msec
service timestamps log datetime msec
no service password-encryption
!
hostname qa-device
!
boot-start-marker
boot-end-marker
!
ip domain name lab.local
ip cef
no ipv6 cef
!
multilink bundle-name authenticated
!
!
!
!
username cisco privilege 15 secret 9 <redacted secret>
!
redundancy
!`

func newTestProfile() *NCMProfile {
	return &NCMProfile{
		Name: "test",
		Commands: CommandSet{
			GetRunning: &PlainCommand{
				Command: "show running-config",
				Validator: Validator{
					Require: []*regexp.Regexp{
						regexp.MustCompile("Building configuration..."),
					},
				},
			},
		},
		MetadataRules: []MetadataRule{
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`! Last configuration change at (.*)`),
				Format: "15:04:05 MST Mon Jan 2 2006",
			},
			{
				Type:  ConfigSize,
				Regex: regexp.MustCompile(`Current configuration : (?P<Size>\d+)`),
			},
		},
		Redactions: []RedactionRule{
			{Regex: regexp.MustCompile(`(username .+ (password|secret) \d) .+`), Replacement: "$1 <redacted secret>"},
		},
	}
}

// DefaultProfile extracts the official default profile by name
func DefaultProfile(t testing.TB, profileName ProfileName) *NCMProfile {
	p, ok := DefaultProfiles[profileName]
	if !ok {
		t.Fatalf("Attempted to load nonexistent profile %q", profileName)
	}
	return p
}

// SetProfilesForTesting allows tests to override the profiles map.
func SetProfilesForTesting(t testing.TB, profiles Map) {
	profilesOverride = profiles
	t.Cleanup(func() {
		profilesOverride = nil
	})
}

var TestProfiles = Map{
	"_base": &NCMProfile{
		Name: "base",
	},
	"p1": &NCMProfile{
		Name: "p1",
		Commands: CommandSet{
			Verify:     MkCommand("show sys", Expect(`Test Profile p1`)),
			GetRunning: MkCommand("show run"),
			GetStartup: MkCommand("show start"),
			GetVersion: MkCommand("show ver"),
		},
	},
	"p2": &NCMProfile{
		Name: "p2",
		Commands: CommandSet{
			Verify:     MkCommand("show system", Expect(`System P2`)),
			GetRunning: MkCommand("show running-config", Expect("Building configuration...")),
			GetStartup: MkCommand("show startup-config"),
			GetVersion: MkCommand("show version"),
		},
		Redactions: []RedactionRule{
			MkRedaction("(username .+ (password|secret) \\d) .+", WithReplacement("$1 <redacted secret>")),
		},
		MetadataRules: []MetadataRule{
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`! Last configuration change at (.*)`),
				Format: "15:04:05 MST Mon Jan 2 2006",
			},
			{
				Type:  ConfigSize,
				Regex: regexp.MustCompile(`Current configuration : (?P<Size>\\d+)`),
			},
		},
	},
}
