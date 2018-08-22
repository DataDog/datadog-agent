package procmatch

import "fmt"

type graphMatcher struct {
	graph *signatureGraph
}

func (m graphMatcher) Match(cmdline string) string {
	return m.graph.searchIntegration(cmdline)
}

// NewWithGraph builds a graph matcher from an integration catalog
func NewWithGraph(catalog IntegrationCatalog) (Matcher, error) {
	signatures := buildSignatures(catalog)

	graph, err := buildSignatureGraph(signatures)
	if err != nil {
		return nil, fmt.Errorf("error building the graph: %s", err)
	}

	return graphMatcher{graph}, nil
}
