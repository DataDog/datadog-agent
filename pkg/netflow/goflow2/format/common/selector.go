package common

import (
	"flag"
	"strings"
	"sync"
)

var (
	selectorVar string
	selector    []string // Hashing fields
	selectorMap = make(map[string]bool)

	selectorDeclared     bool
	selectorDeclaredLock = &sync.Mutex{}
)

// SelectorFlag desc
func SelectorFlag() {
	selectorDeclaredLock.Lock()
	defer selectorDeclaredLock.Unlock()

	if selectorDeclared {
		return
	}
	selectorDeclared = true
	flag.StringVar(&selectorVar, "format.selector", "", "List of fields to do keep in output")
}

// ManualSelectorInit desc
func ManualSelectorInit() error {
	if selectorVar == "" {
		return nil
	}
	selector = strings.Split(selectorVar, ",")
	for _, v := range selector {
		selectorMap[v] = true
	}
	return nil
}
