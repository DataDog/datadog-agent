package tagstore

import "time"

// sourceTags holds the tags for a given entity collected from a single source,
// grouped by their cardinality.
type sourceTags struct {
	lowCardTags          []string
	orchestratorCardTags []string
	highCardTags         []string
	standardTags         []string
	expiryDate           time.Time
}

func (st *sourceTags) isEmpty() bool {
	return len(st.lowCardTags) == 0 && len(st.orchestratorCardTags) == 0 && len(st.highCardTags) == 0 && len(st.standardTags) == 0
}

func (st *sourceTags) isExpired(t time.Time) bool {
	if st.expiryDate.IsZero() {
		return false
	}

	return st.expiryDate.Before(t)
}
