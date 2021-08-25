package network

const (
	ephemeralRangeStart = 32768
	ephemeralRangeEnd   = 60999
)

// IsEphemeralPort returns true if a port belongs to the ephemeral range
// This is mostly a placeholder for now as we have work planned for a
// platform-agnostic solution that will, among other things, source these values
// from procfs for Linux hosts
func IsEphemeralPort(port int) bool {
	return port >= ephemeralRangeStart && port <= ephemeralRangeEnd
}
