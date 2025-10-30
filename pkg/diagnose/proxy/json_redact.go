package proxy

import (
	"encoding/json"
)

// make an alias to avoid infinite recursion in MarshalJSON
type valueWithSourceAlias ValueWithSource

func (v ValueWithSource) MarshalJSON() ([]byte, error) {
	alias := valueWithSourceAlias(v)
	// Redact only on output; keep runtime value intact elsewhere.
	alias.Value = RedactURL(v.Value)
	return json.Marshal(alias)
}
