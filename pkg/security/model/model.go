//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/accessors -output ../secl/eval/model_accessors.go

package model

type OpenSyscall struct {
	Filename string `yaml:"filename" field:"filename" tags:"fs"`
	Flags    int    `yaml:"flags" field:"flags" tags:"fs"`
	Mode     int    `yaml:"mode" field:"mode" tags:"fs"`
}

type UnlinkSyscall struct {
	Filename string `yaml:"filename" field:"filename" tags:"fs"`
}

type RenameSyscall struct {
	OldName string `yaml:"oldname" field:"oldname" tags:"fs"`
	NewName string `yaml:"newname" field:"newname" tags:"fs"`
}

type Process struct {
	UID  int    `yaml:"UID" field:"uid" tags:"process"`
	GID  int    `yaml:"GID" field:"gid" tags:"process"`
	PID  int    `yaml:"PID" field:"pid" tags:"process"`
	Name string `yaml:"name" field:"name" tags:"process"`
}

type Container struct {
	ID     string   `yaml:"id" field:"id" tags:"container"`
	Labels []string `yaml:"labels" field:"labels" tags:"container"`
}

// genaccessors
type Event struct {
	Process   Process       `yaml:"process" field:"process"`
	Container Container     `yaml:"container" field:"container"`
	Syscall   string        `yaml:"syscall" field:"syscall"`
	Open      OpenSyscall   `yaml:"open" field:"open"`
	Unlink    UnlinkSyscall `yaml:"unlink" field:"unlink"`
	Rename    RenameSyscall `yaml:"rename" field:"rename"`
}
