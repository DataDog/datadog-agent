// +build linux

package kernel

import (
	"io/ioutil"
	"path/filepath"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type LockdownMode string

const (
	None            LockdownMode = "none"
	Integrity       LockdownMode = "integrity"
	Confidentiality LockdownMode = "confidentiality"
	Unknown         LockdownMode = "unknown"
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

func GetLockdownMode() LockdownMode {
	data, err := ioutil.ReadFile(filepath.Join(util.GetSysRoot(), "kernel/security/lockdown"))
	if err != nil {
		return Unknown
	}

	return getLockdownMode(string(data))
}

func IsLockdownEnabled() bool {
	if mode := GetLockdownMode(); mode != None && mode != Unknown {
		return true
	}
	return false
}
