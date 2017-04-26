package pidfile

import "syscall"

const (
	processQueryLimitedInformation = 0x1000

	stillActive = 259
)

// isProcess checks to see if a given pid is currently valid in the process table
func isProcess(pid int) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	var c uint32
	err = syscall.GetExitCodeProcess(h, &c)
	syscall.Close(h)
	if err != nil {
		return c == stillActive
	}
	return true
}

// Path returns a suitable location for the pidfile under Windows
func Path() string {
	return "c:\\ProgramData\\DataDog\\datadog-agent.pid"
}
