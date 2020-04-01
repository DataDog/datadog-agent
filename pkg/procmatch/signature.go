package procmatch

import (
	"sort"
	"strings"
)

// getSignatures retrieves the signatures structured
func (i IntegrationEntry) getSignatures() []signature {
	sigs := []signature{}

	for _, rawSig := range i.Signatures {
		sigs = append(sigs, signature{
			integration: Integration{DisplayName: i.DisplayName, MetricPrefix: i.MetricPrefix, Name: i.Name},
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
