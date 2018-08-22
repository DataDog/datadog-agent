package procmatch

//go:generate go run gen/generate_catalog.go

// Matcher interface implements the Match method that takes a command line string and returns a potential integration for this command line string
type Matcher interface {
	Match(cmdline string) string // Return the name of the integration
}

// Integration represents an integration
type Integration struct {
	Name       string   // Name of the integration
	Signatures []string // Signatures of the integration's command line processes
}

// IntegrationCatalog represents a list of Integrations
type IntegrationCatalog []Integration

// Default matchers
var defaultGraphMatcher Matcher
var defaultContainsMatcher Matcher

func init() {
	var err error
	defaultGraphMatcher, err = NewWithGraph(DefaultCatalog)

	if err != nil {
		panic(err)
	}

	defaultContainsMatcher = NewWithContains(DefaultCatalog)
}

// Match uses the default matcher (graph one) built with the default catalog
func Match(cmdline string) string {
	return defaultGraphMatcher.Match(cmdline)
}

// MatchWithContains uses the contains matcher built with the default catalog
func MatchWithContains(cmdline string) string {
	return defaultContainsMatcher.Match(cmdline)
}
