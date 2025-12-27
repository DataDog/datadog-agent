/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
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
