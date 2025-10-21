package os

import (
	"fmt"
	"strings"
)

type Architecture string

const (
	AMD64Arch = Architecture("x86_64")
	ARM64Arch = Architecture("arm64")
)

func ArchitectureFromString(archStr string) Architecture {
	archStr = strings.ToLower(archStr)
	switch archStr {
	case "x86_64", "amd64", "", "x86_64_mac": // Default architecture is AMD64
		return AMD64Arch
	case "arm64", "aarch64", "arm64_mac":
		return ARM64Arch
	default:
		panic(fmt.Sprintf("unknown architecture: %s", archStr))
	}
}

type Family int

const (
	UnknownFamily Family = iota

	LinuxFamily   Family = iota
	WindowsFamily Family = iota
	MacOSFamily   Family = iota
)

type Flavor int

const (
	Unknown Flavor = iota

	// Linux
	Ubuntu         Flavor = iota
	AmazonLinux    Flavor = iota
	AmazonLinuxECS Flavor = iota
	Debian         Flavor = iota
	RedHat         Flavor = iota
	Suse           Flavor = iota
	Fedora         Flavor = iota
	CentOS         Flavor = iota
	RockyLinux     Flavor = iota

	// Windows
	WindowsServer Flavor = 500
	WindowsClient Flavor = 501

	// MacOS
	MacosOS Flavor = 1000
)

func FlavorFromString(flavorStr string) Flavor {
	flavorStr = strings.ToLower(flavorStr)
	switch flavorStr {
	case "", "ubuntu": // Default flavor is Ubuntu
		return Ubuntu
	case "amazon-linux", "amazonlinux":
		return AmazonLinux
	case "amazon-linux-ecs", "amazonlinuxecs":
		return AmazonLinuxECS
	case "debian":
		return Debian
	case "redhat":
		return RedHat
	case "suse":
		return Suse
	case "fedora":
		return Fedora
	case "centos":
		return CentOS
	case "rocky-linux", "rockylinux":
		return RockyLinux
	case "windows", "windows-server":
		return WindowsServer
	case "windows-client":
		return WindowsClient
	case "macos":
		return MacosOS
	default:
		panic(fmt.Sprintf("unknown OS flavor: %s", flavorStr))
	}
}

func (f Flavor) Type() Family {
	switch {
	case f < WindowsServer:
		return LinuxFamily
	case f < MacosOS:
		return WindowsFamily
	case f == MacosOS:
		return MacOSFamily
	default:
		panic("unknown OS flavor")
	}
}

func (f Flavor) String() string {
	switch f {
	case Ubuntu:
		return "ubuntu"
	case AmazonLinux:
		return "amazon-linux"
	case AmazonLinuxECS:
		return "amazon-linux-ecs"
	case Debian:
		return "debian"
	case RedHat:
		return "redhat"
	case Suse:
		return "suse"
	case Fedora:
		return "fedora"
	case CentOS:
		return "centos"
	case RockyLinux:
		return "rocky-linux"
	case WindowsServer:
		return "windows-server"
	case WindowsClient:
		return "windows-client"
	case MacosOS:
		return "macos"
	case Unknown:
		fallthrough
	default:
		panic("unknown OS flavor")
	}
}
