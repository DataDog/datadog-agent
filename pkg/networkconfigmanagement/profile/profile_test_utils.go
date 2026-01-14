// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package profile

import (
	"embed"
	"fmt"
	"path"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

//go:embed fixtures/*
var fixturesFS embed.FS

// Fixture represents the data to pass in for the test and its expected output for profile definitions
type Fixture struct {
	Initial  []byte
	Expected []byte
}

func loadFixture(profileName string, command CommandType) Fixture {
	initialPath := path.Join("fixtures", profileName, string(command), "initial.txt")
	initial, err := fixturesFS.ReadFile(initialPath)
	if err != nil {
		panic(fmt.Sprintf("could not load initial data fixture for profile: %s, command: %s, error: %s", profileName, command, err))
	}
	expectedPath := path.Join("fixtures", profileName, string(command), "expected.txt")
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
username cisco privilege 15 secret 9 <BIG OL SECRET>
!         
redundancy
!`

func newTestProfile() *NCMProfile {
	return &NCMProfile{
		BaseProfile: BaseProfile{
			Name: "test",
		},
		Commands: map[CommandType]*Commands{
			Running: {
				CommandType: Running,
				Values:      []string{"show running-config"},
				ProcessingRules: ProcessingRules{
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
					ValidationRules: []ValidationRule{
						{
							Pattern: regexp.MustCompile("Building configuration..."),
						},
					},
					RedactionRules: []RedactionRule{
						{Regex: regexp.MustCompile(`(username .+ (password|secret) \d) .+`), Replacement: "$1 <BIG OL SECRET>"},
					},
				},
			},
		},
	}
}

var testProfile = &NCMProfile{
	BaseProfile: BaseProfile{
		Name: "test",
	},
	Commands: map[CommandType]*Commands{
		Running: {
			CommandType: Running,
			Values:      []string{"show running-config"},
			ProcessingRules: ProcessingRules{
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
				ValidationRules: []ValidationRule{
					{
						Type:    "valid_output",
						Pattern: regexp.MustCompile("Building configuration..."),
					},
				},
				RedactionRules: []RedactionRule{
					{Regex: regexp.MustCompile(`(username .+ (password|secret) \d) .+`), Replacement: "$1 <redacted secret>"},
				},
			},
		},
	},
}

var runningCommandsWithCompiledRegex = &Commands{
	CommandType: Running,
	Values:      []string{"show running-config"},
	ProcessingRules: ProcessingRules{
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
		ValidationRules: []ValidationRule{
			{
				Type:    "valid_output",
				Pattern: regexp.MustCompile(`Building configuration...`),
			},
		},
		RedactionRules: []RedactionRule{
			{
				Regex:       regexp.MustCompile(`(username .+ (password|secret) \d) .+`),
				Replacement: `$1 <redacted secret>`,
			},
		},
	},
	Scrubber: getRunningScrubber(),
}

func getRunningScrubber() *scrubber.Scrubber {
	sc := scrubber.New()
	sc.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex: regexp.MustCompile(`(username .+ (password|secret) \d) .+`),
		Repl:  []byte("$1 " + "<redacted secret>"),
	})
	return sc
}

// DefaultProfile will parse the official default profile given the name of the profile file
func DefaultProfile(profileName string) *NCMProfile {
	file := profileName + ".json"
	b, _ := defaultProfilesFS.ReadFile(path.Join(defaultProfilesFolder, file))
	prof, _ := parseNCMProfileFromBytes(b, profileName)
	return prof
}

// IOSProfile parses the test profile for IOS devices to test with
func IOSProfile() *NCMProfile {
	b, _ := defaultProfilesFS.ReadFile(path.Join(defaultProfilesFolder, "cisco-ios.json"))
	prof, _ := parseNCMProfileFromBytes(b, "cisco-ios")
	return prof
}

// JunOSProfile parses the test profile for junOS devices to test with
func JunOSProfile() *NCMProfile {
	b, _ := defaultProfilesFS.ReadFile(path.Join(defaultProfilesFolder, "junos.json"))
	prof, _ := parseNCMProfileFromBytes(b, "junos")
	return prof
}
