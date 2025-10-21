package utils

import (
	"encoding/json"
)

func JSONMustMarshal(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}
