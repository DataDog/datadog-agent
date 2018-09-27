package procmatch

//go:generate go run gen/generate_catalog.go

// Matcher interface implements the Match method that takes a command line string and returns a potential integration for this command line string
type Matcher interface {
	Match(cmdline string) Integration // Return the matched integration
}

// Integration represents an integration
type Integration struct {
	MetricPrefix string // Metric prefix of the integration
	Name         string // Name of the integration
	DisplayName  string // DisplayName of the integration
}

// IntegrationEntry represents an integration entry in the catalog
type IntegrationEntry struct {
	MetricPrefix string   // Metric prefix of the integration
	Name         string   // Name of the integration
	DisplayName  string   // DisplayName of the integration
	Signatures   []string // Signatures of the integration's command line processes
}

// IntegrationCatalog represents a list of Integrations
type IntegrationCatalog []IntegrationEntry

type signature struct {
	integration Integration
	words       []string
}

// NewDefault returns the default matcher (graph one) built with the default catalog
func NewDefault() (Matcher, error) {
	return NewMatcher(DefaultCatalog)
}
