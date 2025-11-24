// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"fmt"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// Handles AMIs for all OSes

// map[os][arch][version] = ami (e.g. map[ubuntu][x86_64][22.04] = "ami-01234567890123456")
type PlatformsAMIsType = map[string]string
type PlatformsArchsType = map[string]PlatformsAMIsType
type PlatformsType = map[string]PlatformsArchsType

// All the OS descriptors / AMIs correspondance
var platforms = PlatformsType{
	"debian": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"9":  "ami-0182559468c1975fe",
			"10": "ami-06af96db6765fa419",
			"11": "ami-0c552d040ce3b66e0",
			"12": "ami-0531f78dcce082188",
		},
		"arm64": PlatformsAMIsType{
			"10": "ami-0b1f3e4ea76c9a5d2",
			"11": "ami-0917ebceb11be2e8a",
			"12": "ami-04935cdb518e4a366",
		},
	},
	"ubuntu": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"14-04": "ami-013d633d3b6cdb22c",
			"16-04": "ami-0296699e24cbb4143",
			"18-04": "ami-0b523f7e6425e2c8c",
			"20-04": "ami-0c41f56c881c68720",
			"22-04": "ami-0e3913528c47ae3f5",
			"23-04": "ami-0533a8aa5b8ad7c7c",
			"23-10": "ami-0949b45ef274e55a1",
			"24-04": "ami-02fb31d76d5fa8d35",
		},
		"arm64": PlatformsAMIsType{
			"18-04":   "ami-0e07da45fcfc0c4c9",
			"20-04":   "ami-06a75cdeb2f77ae18",
			"20-04-2": "ami-023f1e40b096c3ebc",
			"21-04":   "ami-019b0f2a791d2bbc0",
			"22-04":   "ami-07b7d55284f346a96",
			"23-04":   "ami-0b32303b545cba4db",
			"23-10":   "ami-0dea732dd5f1da0a8",
			"24-04":   "ami-0a8a1016b19572af2",
		},
	},
	"amazon-linux": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"2-4-14":    "ami-038b3df3312ddf25d",
			"2-5-10":    "ami-06a0cd9728546d178",
			"2022-5-15": "ami-0f0f00c2d082c52ae",
			"2023":      "ami-058a5d626099cf16c",
			"2018":      "ami-02caf923ff4b59e8c",
			"2":         "ami-0da449b9f8245c763",
		},
		"arm64": PlatformsAMIsType{
			"2-4-14":    "ami-090230ed0c6b13c74",
			"2-5-10":    "ami-09e51988f56677f44",
			"2022-5-15": "ami-0acc51c3357f26240",
			"2023":      "ami-0aaec32babf1e822c",
			"2":         "ami-04ede3126a4d26b8e",
		},
	},
	"amazon-linux-ecs": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"2023": "ami-0d6b553b43813a487",
			"2":    "ami-0434f20bd534b3f1d",
		},
		"arm64": PlatformsAMIsType{
			"2023": "ami-0ac7ab05b219bc7f2",
			"2":    "ami-0293ff221e87260aa",
		},
	},
	"redhat": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"9":       "ami-04b5ab048661f55a5",
			"86":      "ami-031eff1ae75bb87e4",
			"86-fips": "ami-0d0fb96b595c56e03",
		},
		"arm64": PlatformsAMIsType{
			"9":  "ami-05ae470afd4fb9dd6",
			"86": "ami-0238411fb452f8275",
		},
	},
	"suse": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"12":   "ami-09fce481fa7fd07b3",
			"15-4": "ami-0128c2ff9ed3d3978",
		},
		"arm64": PlatformsAMIsType{
			"15-4": "ami-02cf9b47af54755f4",
		},
	},
	"fedora": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"40": "ami-0852a7102687480c5",
		},
		"arm64": PlatformsAMIsType{
			"42": "ami-0953cc8b868ba599d",
		},
	},
	"centos": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"610": "ami-0506f01ccb6dddeda",
			"79":  "ami-036de472bb001ae9c",
		},
		"arm64": PlatformsAMIsType{
			"79": "ami-0cb7a00afccf30559",
		},
	},
	"rocky-linux": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"92": "ami-08f362c39d03a4eb5",
		},
	},
	"windows-server": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"2025": "ami-046960a099673d97d",
			"2022": "ami-001eb8f77f0589d4b",
			"2019": "ami-0144cf01bbf315e1b",
			"2016": "ami-0bab8643e749a877f",
		},
	},
	"macos": PlatformsArchsType{
		"arm64": PlatformsAMIsType{
			"sonoma": "ami-04c4082c98a93544e",
		},
		"x86_64": PlatformsAMIsType{
			"sonoma": "ami-0801764b08904a0e4",
		},
	},
}

func GetAMI(descriptor *e2eos.Descriptor) (string, error) {
	if _, ok := platforms[descriptor.Flavor.String()]; !ok {
		return "", fmt.Errorf("os '%s' not found in platforms map", descriptor.Flavor.String())
	}
	if _, ok := platforms[descriptor.Flavor.String()][string(descriptor.Architecture)]; !ok {
		return "", fmt.Errorf("arch '%s' not found in platforms map", descriptor.Architecture)
	}
	if _, ok := platforms[descriptor.Flavor.String()][string(descriptor.Architecture)][descriptor.Version]; !ok {
		return "", fmt.Errorf("version '%s' not found in platforms map", descriptor.Version)
	}

	return platforms[descriptor.Flavor.String()][string(descriptor.Architecture)][descriptor.Version], nil
}
