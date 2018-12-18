package procmatch

import "fmt"

type graphMatcher struct {
	graph *signatureGraph
}

func (m graphMatcher) Match(cmdline string) Integration {
	return m.graph.searchIntegration(cmdline)
}

// NewMatcher builds a graph matcher from an integration catalog
func NewMatcher(catalog IntegrationCatalog) (Matcher, error) {
	signatures := buildSignatures(catalog)

	graph, err := buildSignatureGraph(signatures)
	if err != nil {
		return nil, fmt.Errorf("error building the graph: %s", err)
	}

	return graphMatcher{graph}, nil
}
