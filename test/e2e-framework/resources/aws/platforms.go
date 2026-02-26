// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"fmt"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// PlatformsAMIsType maps version to AMI ID (e.g. map["22.04"] = "ami-01234567890123456").
type PlatformsAMIsType = map[string]string

// PlatformsArchsType maps architecture to version-AMI mappings.
type PlatformsArchsType = map[string]PlatformsAMIsType

// PlatformsType maps OS flavor to architecture-version-AMI mappings.
type PlatformsType = map[string]PlatformsArchsType

// All the OS descriptors / AMIs correspondance
var platforms = PlatformsType{
	"debian": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"9":  "ami-0182559468c1975fe",
			"10": "ami-0c12641b3d96f5864",
			"11": "ami-0292c712052a3bb7e",
			"12": "ami-0e406f95f6e2ded1a",
		},
		"arm64": PlatformsAMIsType{
			"10": "ami-002b2963e972b25be",
			"11": "ami-0764e0908076793da",
			"12": "ami-0e9d9a9c7e58d338c",
		},
	},
	"ubuntu": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"14-04": "ami-013d633d3b6cdb22c",
			"16-04": "ami-0bb031a78a02feb49",
			"18-04": "ami-0678469d5c9f144d1",
			"20-04": "ami-0edd78b5d819ec15c",
			"22-04": "ami-027500b5d945a08e4",
			"23-04": "ami-024efd3acb5a492ad",
			"23-10": "ami-0949b45ef274e55a1",
			"24-04": "ami-0e4b71849cb24823e",
		},
		"arm64": PlatformsAMIsType{
			"18-04":   "ami-099d4eba778eb0b07",
			"20-04":   "ami-0bd112783003112dd",
			"20-04-2": "ami-023f1e40b096c3ebc",
			"21-04":   "ami-0cd8fed7ad851b749",
			"22-04":   "ami-0a3c12a2df75e571f",
			"23-04":   "ami-0fe18d167cc1beefa",
			"23-10":   "ami-0dea732dd5f1da0a8",
			"24-04":   "ami-0f20c84e35c1b03f4",
		},
	},
	"amazon-linux": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"2-4-14":    "ami-038b3df3312ddf25d",
			"2-5-10":    "ami-06a0cd9728546d178",
			"2022-5-15": "ami-0f0f00c2d082c52ae",
			"2023":      "ami-094b2da2cec5987d5",
			"2018":      "ami-03f8ea51a1431a1f5",
			"2":         "ami-0f0116413cfdc9819",
		},
		"arm64": PlatformsAMIsType{
			"2-4-14":    "ami-090230ed0c6b13c74",
			"2-5-10":    "ami-09e51988f56677f44",
			"2022-5-15": "ami-0acc51c3357f26240",
			"2023":      "ami-06a8357bdfd9c8167",
			"2":         "ami-01d2ed2772d47ca5e",
		},
	},
	"amazon-linux-ecs": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"2023": "ami-0568a9598e25ab865",
			"2":    "ami-0e200a3844e935535",
		},
		"arm64": PlatformsAMIsType{
			"2023": "ami-079739e4395e8c3df",
			"2":    "ami-01557b18094656413",
		},
	},
	"redhat": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"9":       "ami-0030ca6c2fb82df51",
			"86":      "ami-031eff1ae75bb87e4",
			"86-fips": "ami-0d0fb96b595c56e03",
		},
		"arm64": PlatformsAMIsType{
			"9":  "ami-0fe660a31ee380e65",
			"86": "ami-0238411fb452f8275",
		},
	},
	"suse": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"12":   "ami-0859156816dcf83f3",
			"15-4": "ami-0519e920b3b7fd800",
		},
		"arm64": PlatformsAMIsType{
			"15-4": "ami-0656be89a9cbf6a4d",
		},
	},
	"fedora": PlatformsArchsType{
		"x86_64": PlatformsAMIsType{
			"40": "ami-01cd99928e0a0a126",
		},
		"arm64": PlatformsAMIsType{
			"42": "ami-0b0735946c40cdbd5",
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
			"2025": "ami-00a04c74037c525a8",
			"2022": "ami-05f0557868dde10c5",
			"2019": "ami-0940cc5f5a114af9d",
			"2016": "ami-01533cd711f9865d0",
		},
	},
	"macos": PlatformsArchsType{
		"arm64": PlatformsAMIsType{
			"sonoma": "ami-08b9fed5412fba1a2",
		},
		"x86_64": PlatformsAMIsType{
			"sonoma": "ami-076d450c7e01e70c8",
		},
	},
}

// GetAMI returns the AMI ID for the given OS descriptor from the hardcoded platforms map.
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
