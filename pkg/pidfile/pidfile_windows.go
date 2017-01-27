package pidfile

// BUG(massi): This needs to be implemented
func isProcess(pid int) bool {
	return false
}

// Path returns a suitable location for the pidfile under Windows
//
// BUG(massi): This needs to be implemented
func Path() string {
	return ""
}
