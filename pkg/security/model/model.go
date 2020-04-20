//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/accessors -output ../secl/eval/model_accessors.go

package model

type OpenSyscall struct {
	Pathname string `yaml:"pathname" field:"pathname"`
	Flags    int    `yaml:"flags" field:"flags"`
	Mode     int    `yaml:"mode" field:"mode"`
}

type UnlinkSyscall struct {
	Pathname string `yaml:"pathname" field:"pathname"`
}

type RenameSyscall struct {
	OldName string `yaml:"oldname" field:"oldname"`
	NewName string `yaml:"newname" field:"newname"`
}

type Process struct {
	UID  int    `yaml:"UID" field:"uid"`
	GID  int    `yaml:"GID" field:"gid"`
	PID  int    `yaml:"PID" field:"pid"`
	Name string `yaml:"name" field:"name"`
}

type Container struct {
	ID     string   `yaml:"id" field:"id"`
	Labels []string `yaml:"labels" field:"labels"`
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
