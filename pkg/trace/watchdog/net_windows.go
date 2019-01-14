package watchdog

// Net for windows returns basic network info without the number of connections.
func (pi *CurrentInfo) Net() NetInfo {
	return NetInfo{}
}
