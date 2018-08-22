package procmatch

import (
	"sort"
	"strings"
)

type signature struct {
	integration string
	words       []string
}

// getSignatures retrieves the signatures structured
func (i Integration) getSignatures() []signature {
	sigs := []signature{}

	for _, rawSig := range i.Signatures {
		sigs = append(sigs, signature{
			integration: i.Name,
			words:       strings.FieldsFunc(strings.ToLower(rawSig), splitCmdline),
		})
	}

	return sigs
}

// buildSignatures builds the signatures from the given integration catalog and returns them sorted by length of words
func buildSignatures(integs IntegrationCatalog) []signature {
	signatures := []signature{}

	for _, integ := range integs {
		signatures = append(signatures, integ.getSignatures()...)
	}

	// We must match the more specific signatures first. So if a process cmdline matches more than
	// one integration, we'll return the most specific one i.e. the one with more words
	sort.SliceStable(signatures, func(i, j int) bool {
		return len(signatures[i].words) > len(signatures[j].words)
	})

	return signatures
}

// Returns true if the given commandline matches the signature
func (s *signature) match(cmdline string) bool {
	i := 0
	for _, f := range s.words {
		if j := strings.Index(cmdline[i:], f); j != -1 {
			i += j + len(f)
			continue
		}
		return false
	}
	return true
}
