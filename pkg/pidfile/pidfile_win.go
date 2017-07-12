package pidfile



// isProcess uses `kill -0` to check whether a process is running
func isProcess(pid int) bool {
	return false
}

// Path returns a suitable location for the pidfile under OSX
func Path() string {
	return ""
}
