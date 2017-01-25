package pidfile

func isProcess(pid int) bool {
	return false
}

// Path returns a suitable location for the pidfile under OSX
func Path() string {
	return ""
}
