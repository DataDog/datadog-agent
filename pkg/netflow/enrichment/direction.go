package enrichment

// RemapDirection remaps direction from 0 or 1 to respectively ingress or egress
func RemapDirection(direction uint32) string {
	if direction == 1 {
		return "egress"
	}
	return "ingress"
}
