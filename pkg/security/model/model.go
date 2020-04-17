//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/accessors -output ../secl/eval/model_accessors.go

package model

type OpenSyscall struct {
	Pathname string `yaml:"pathname"`
	Flags    int    `yaml:"flags"`
	Mode     int    `yaml:"mode"`
}

type UnlinkSyscall struct {
	Pathname string `yaml:"pathname"`
}

type RenameSyscall struct {
	OldName string `yaml:"oldname"`
	NewName string `yaml:"newname"`
}

type Process struct {
	UID  int    `yaml:"UID"`
	GID  int    `yaml:"GID"`
	PID  int    `yaml:"PID"`
	Name string `yaml:"name"`
}

type Container struct {
	ID     string   `yaml:"id"`
	Labels []string `yaml:"labels"`
}

// genaccessors
type Event struct {
	Process   Process       `yaml:"process"`
	Container Container     `yaml:"container"`
	Syscall   string        `yaml:"syscall"`
	Open      OpenSyscall   `yaml:"open"`
	Unlink    UnlinkSyscall `yaml:"unlink"`
	Rename    RenameSyscall `yaml:"rename"`
}
