// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package profile

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

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
							Regex:  `! Last configuration change at (.*)`,
							Format: "15:04:05 MST Mon Jan 2 2006",
						},
						{
							Type:  ConfigSize,
							Regex: `Current configuration : (?P<Size>\d+)`,
						},
					},
					ValidationRules: []ValidationRule{
						{
							Pattern: "Building configuration...",
						},
					},
					RedactionRules: []RedactionRule{
						{Regex: `(username .+ (password|secret) \d) .+`, Replacement: "<BIG OL SECRET>"},
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
						Regex:  `! Last configuration change at (.*)`,
						Format: "15:04:05 MST Mon Jan 2 2006",
					},
					{
						Type:  ConfigSize,
						Regex: `Current configuration : (?P<Size>\d+)`,
					},
				},
				ValidationRules: []ValidationRule{
					{
						Type:    "valid_output",
						Pattern: "Building configuration...",
					},
				},
				RedactionRules: []RedactionRule{
					{Regex: `(username .+ (password|secret) \d) .+`, Replacement: "<redacted secret>"},
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
				Regex:  `! Last configuration change at (.*)`,
				Regexp: regexp.MustCompile(`! Last configuration change at (.*)`),
				Format: "15:04:05 MST Mon Jan 2 2006",
			},
			{
				Type:   ConfigSize,
				Regex:  `Current configuration : (?P<Size>\d+)`,
				Regexp: regexp.MustCompile(`Current configuration : (?P<Size>\d+)`),
			},
		},
		ValidationRules: []ValidationRule{
			{
				Type:    "valid_output",
				Pattern: "Building configuration...",
				Regexp:  regexp.MustCompile(`Building configuration...`),
			},
		},
		RedactionRules: []RedactionRule{
			{
				Regex:       `(username .+ (password|secret) \d) .+`,
				Regexp:      regexp.MustCompile(`(username .+ (password|secret) \d) .+`),
				Replacement: `<redacted secret>`,
			},
		},
	},
	Scrubber: getRunningScrubber(),
}

func getRunningScrubber() *scrubber.Scrubber {
	sc := scrubber.New()
	sc.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex: regexp.MustCompile(`(username .+ (password|secret) \d) .+`),
		Repl:  []byte(fmt.Sprintf(`$1 %s`, "<redacted secret>")),
	})
	return sc
}
