// +build linux

package kernel

import (
	"io/ioutil"
	"path/filepath"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// LockdownMode defines a lockdown type
type LockdownMode string

const (
	// None mode
	None LockdownMode = "none"
	// Integrity mode
	Integrity LockdownMode = "integrity"
	// Confidentiality mode
	Confidentiality LockdownMode = "confidentiality"
	// Unknown mode
	Unknown LockdownMode = "unknown"
)

var re = regexp.MustCompile(`\[(.*)\]`)

func getLockdownMode(data string) LockdownMode {
	mode := re.FindString(data)

	switch mode {
	case "[none]":
		return None
	case "[integrity]":
		return Integrity
	case "[confidentiality]":
		return Confidentiality
	}
	return Unknown
}

// GetLockdownMode returns the lockdown
func GetLockdownMode() LockdownMode {
	data, err := ioutil.ReadFile(filepath.Join(util.GetSysRoot(), "kernel/security/lockdown"))
	if err != nil {
		return Unknown
	}

	return getLockdownMode(string(data))
}
