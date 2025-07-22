package strings

import (
	"unique"
)

// ToUnique converts a slice of strings to a slice of interned strings.
func ToUnique(ss []string) []unique.Handle[string] {
	if ss == nil {
		return nil
	}
	ret := make([]unique.Handle[string], 0, len(ss))
	for _, s := range ss {
		ret = append(ret, unique.Make(s))
	}
	return ret
}

// FromUnique converts a slice of unique string handles into a slice of strings.
func FromUnique(ss []unique.Handle[string]) []string {
	if ss == nil {
		return nil
	}
	ret := make([]string, 0, len(ss))
	for _, s := range ss {
		ret = append(ret, s.Value())
	}
	return ret
}
