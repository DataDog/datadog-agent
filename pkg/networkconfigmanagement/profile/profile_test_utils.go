// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package profile

import (
	"embed"
	"fmt"
	"path/filepath"
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

func loadFixture(profileName string) Fixture {
	initial, err := fixturesFS.ReadFile("fixtures/" + profileName + "/initial.txt")
	if err != nil {
		panic(fmt.Sprintf("could not load initial data fixture for profile: %s, error: %s", profileName, err))
	}
	expected, err := fixturesFS.ReadFile("fixtures/" + profileName + "/expected.txt")
	if err != nil {
		panic(fmt.Sprintf("could not load expected data fixture for profile: %s, error: %s", profileName, err))
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

var ncmDefaultProfilesPath = filepath.Join("..", "..", "..", "cmd", "agent", "dist", "conf.d", "network_config_management.d", "default_profiles")

// IOSProfile parses the test profile for IOS devices to test with
func IOSProfile() *NCMProfile {
	file, _ := filepath.Abs(filepath.Join(ncmDefaultProfilesPath, "cisco-ios.json"))
	configFile := resolveNCMProfileDefinitionPath(file)
	prof, _ := ParseNCMProfileFromFile(configFile)
	return prof
}

// JunOSProfile parses the test profile for junOS devices to test with
func JunOSProfile() *NCMProfile {
	file, _ := filepath.Abs(filepath.Join(ncmDefaultProfilesPath, "junos.json"))
	configFile := resolveNCMProfileDefinitionPath(file)
	prof, _ := ParseNCMProfileFromFile(configFile)
	return prof
}
