package probe

import (
	"errors"
	"strings"
)

type PolicyMode uint8
type PolicyFlag uint8

const (
	POLICY_MODE_ACCEPT PolicyMode = 1
	POLICY_MODE_DENY   PolicyMode = 2

	BASENAME_FLAG    PolicyFlag = 1
	FLAGS_FLAG       PolicyFlag = 2
	MODE_FLAG        PolicyFlag = 4
	PARENT_NAME_FLAG PolicyFlag = 8
	PROCESS_INODE    PolicyFlag = 16
	PROCESS_NAME     PolicyFlag = 32

	// need to be aligned with the kernel size
	BASENAME_FILTER_SIZE = 32
)

func (m PolicyMode) MarshalJSON() ([]byte, error) {
	switch m {
	case POLICY_MODE_ACCEPT:
		return []byte(`"accept"`), nil
	case POLICY_MODE_DENY:
		return []byte(`"deny"`), nil
	default:
		return nil, errors.New("invalid policy mode")
	}
}

func (f PolicyFlag) MarshalJSON() ([]byte, error) {
	var flags []string
	if f&BASENAME_FLAG != 0 {
		flags = append(flags, `"basename"`)
	}
	if f&FLAGS_FLAG != 0 {
		flags = append(flags, `"flags"`)
	}
	if f&MODE_FLAG != 0 {
		flags = append(flags, `"mode"`)
	}
	if f&PARENT_NAME_FLAG != 0 {
		flags = append(flags, `"name"`)
	}
	if f&PROCESS_INODE != 0 {
		flags = append(flags, `"inode"`)
	}
	if f&PROCESS_NAME != 0 {
		flags = append(flags, `"name"`)
	}
	return []byte("[" + strings.Join(flags, ",") + "]"), nil
}
