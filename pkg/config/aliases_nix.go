//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || aix

package config

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	DefaultSecurityAgentLogFile = pkgconfigsetup.DefaultSecurityAgentLogFile
	DefaultProcessAgentLogFile  = pkgconfigsetup.DefaultProcessAgentLogFile
	DefaultDDAgentBin           = pkgconfigsetup.DefaultDDAgentBin
)
