package configsetup

// DataType represent the generic data type (e.g. metrics, logs) that can be sent by the Agent
type DataType string

const (
	// Metrics type covers series & sketches
	Metrics DataType = "metrics"
	// Logs type covers all outgoing logs
	Logs DataType = "logs"
)

// MappingProfile represent a group of mappings
type MappingProfile struct {
	Name     string          `mapstructure:"name" json:"name" yaml:"name"`
	Prefix   string          `mapstructure:"prefix" json:"prefix" yaml:"prefix"`
	Mappings []MetricMapping `mapstructure:"mappings" json:"mappings" yaml:"mappings"`
}

// MetricMapping represent one mapping rule
type MetricMapping struct {
	Match     string            `mapstructure:"match" json:"match" yaml:"match"`
	MatchType string            `mapstructure:"match_type" json:"match_type" yaml:"match_type"`
	Name      string            `mapstructure:"name" json:"name" yaml:"name"`
	Tags      map[string]string `mapstructure:"tags" json:"tags" yaml:"tags"`
}

// Endpoint represent a datadog endpoint
type Endpoint struct {
	Site   string `mapstructure:"site" json:"site" yaml:"site"`
	URL    string `mapstructure:"url" json:"url" yaml:"url"`
	APIKey string `mapstructure:"api_key" json:"api_key" yaml:"api_key"`
	APPKey string `mapstructure:"app_key" json:"app_key" yaml:"app_key"`
}
