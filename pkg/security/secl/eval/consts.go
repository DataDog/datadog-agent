package eval

import "syscall"

var (
	constants = map[string]interface{}{
		// boolean
		"true":  &BoolEvaluator{Value: true},
		"false": &BoolEvaluator{Value: false},

		// open flags
		"O_RDONLY": &IntEvaluator{Value: syscall.O_RDONLY},
		"O_WRONLY": &IntEvaluator{Value: syscall.O_WRONLY},
		"O_RDWR":   &IntEvaluator{Value: syscall.O_RDWR},
		"O_APPEND": &IntEvaluator{Value: syscall.O_APPEND},
		"O_CREAT":  &IntEvaluator{Value: syscall.O_CREAT},
		"O_EXCL":   &IntEvaluator{Value: syscall.O_EXCL},
		"O_SYNC":   &IntEvaluator{Value: syscall.O_SYNC},
		"O_TRUNC":  &IntEvaluator{Value: syscall.O_TRUNC},

		// permissions
		"S_IEXEC":  &IntEvaluator{Value: syscall.S_IEXEC},
		"S_IFBLK":  &IntEvaluator{Value: syscall.S_IFBLK},
		"S_IFCHR":  &IntEvaluator{Value: syscall.S_IFCHR},
		"S_IFDIR":  &IntEvaluator{Value: syscall.S_IFDIR},
		"S_IFIFO":  &IntEvaluator{Value: syscall.S_IFIFO},
		"S_IFLNK":  &IntEvaluator{Value: syscall.S_IFLNK},
		"S_IFMT":   &IntEvaluator{Value: syscall.S_IFMT},
		"S_IFREG":  &IntEvaluator{Value: syscall.S_IFREG},
		"S_IFSOCK": &IntEvaluator{Value: syscall.S_IFSOCK},
		"S_IREAD":  &IntEvaluator{Value: syscall.S_IREAD},
		"S_IRGRP":  &IntEvaluator{Value: syscall.S_IRGRP},
		"S_IROTH":  &IntEvaluator{Value: syscall.S_IROTH},
		"S_IRUSR":  &IntEvaluator{Value: syscall.S_IRUSR},
		"S_IRWXG":  &IntEvaluator{Value: syscall.S_IRWXG},
		"S_IRWXO":  &IntEvaluator{Value: syscall.S_IRWXO},
		"S_IRWXU":  &IntEvaluator{Value: syscall.S_IRWXU},
		"S_ISGID":  &IntEvaluator{Value: syscall.S_ISGID},
		"S_ISUID":  &IntEvaluator{Value: syscall.S_ISUID},
		"S_ISVTX":  &IntEvaluator{Value: syscall.S_ISVTX},
		"S_IWGRP":  &IntEvaluator{Value: syscall.S_IWGRP},
		"S_IWOTH":  &IntEvaluator{Value: syscall.S_IWOTH},
		"S_IWRITE": &IntEvaluator{Value: syscall.S_IWRITE},
		"S_IWUSR":  &IntEvaluator{Value: syscall.S_IWUSR},
		"S_IXGRP":  &IntEvaluator{Value: syscall.S_IXGRP},
		"S_IXOTH":  &IntEvaluator{Value: syscall.S_IXOTH},
		"S_IXUSR":  &IntEvaluator{Value: syscall.S_IXUSR},
	}
)
