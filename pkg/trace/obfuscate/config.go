package obfuscate

import "github.com/DataDog/datadog-go/statsd"

// Config holds the configuration for obfuscating sensitive data
// for various span types.
type Config struct {
	// ES holds the obfuscation configuration for ElasticSearch bodies.
	ES JSONConfig `mapstructure:"elasticsearch"`

	// Mongo holds the obfuscation configuration for MongoDB queries.
	Mongo JSONConfig `mapstructure:"mongodb"`

	// SQLExecPlan holds the obfuscation configuration for SQL Exec Plans. This is strictly for safety related obfuscation,
	// not normalization. Normalization of exec plans is configured in SQLExecPlanNormalize.
	SQLExecPlan JSONConfig `mapstructure:"sql_exec_plan"`

	// SQLExecPlanNormalize holds the normalization configuration for SQL Exec Plans.
	SQLExecPlanNormalize JSONConfig `mapstructure:"sql_exec_plan_normalize"`

	// SQL specifies additonal SQL configuration options.
	SQL SQLConfig `mapstructure:"-"`

	// HTTP holds the obfuscation settings for HTTP URLs.
	HTTP HTTPConfig `mapstructure:"http"`

	// RemoveStackTraces specifies whether stack traces should be removed.
	// More specifically "error.stack" tag values will be cleared.
	RemoveStackTraces bool `mapstructure:"remove_stack_traces"`

	// Redis holds the configuration for obfuscating the "redis.raw_command" tag
	// for spans of type "redis".
	Redis Enablable `mapstructure:"redis"`

	// Memcached holds the configuration for obfuscating the "memcached.command" tag
	// for spans of type "memcached".
	Memcached Enablable `mapstructure:"memcached"`

	// Statsd specifies the statsd client to use when reporting metrics.
	Statsd statsd.ClientInterface

	// ErrorLogger specifies the logger to use when logging errors.
	Log Logger
}

// SQLConfig specifies the configuration for SQL obfuscation.
type SQLConfig struct {
	// Cache reports whether SQL query obfuscation result caching should be enabled.
	Cache bool

	// TableNames enables adding SQL table names unto spans in the "sql.tables" tag.
	TableNames bool

	// QuantizeTables enables quantiation of table names.
	QuantizeTables bool
}

type noOpLogger struct{}

func (noOpLogger) Errorf(_ string, _ ...interface{}) error { return nil }
func (noOpLogger) Debugf(_ string, _ ...interface{})       {}

// Logger ...
type Logger interface {
	Errorf(format string, params ...interface{}) error
	Debugf(format string, params ...interface{})
}

// HTTPConfig holds the configuration settings for HTTP obfuscation.
type HTTPConfig struct {
	// RemoveQueryStrings determines query strings to be removed from HTTP URLs.
	RemoveQueryString bool `mapstructure:"remove_query_string" json:"remove_query_string"`

	// RemovePathDigits determines digits in path segments to be obfuscated.
	RemovePathDigits bool `mapstructure:"remove_paths_with_digits" json:"remove_path_digits"`
}

// Enablable can represent any option that has an "enabled" boolean sub-field.
type Enablable struct {
	Enabled bool `mapstructure:"enabled"`
}

// JSONConfig holds the obfuscation configuration for sensitive
// data found in JSON objects.
type JSONConfig struct {
	// Enabled will specify whether obfuscation should be enabled.
	Enabled bool `mapstructure:"enabled"`

	// KeepValues will specify a set of keys for which their values will
	// not be obfuscated.
	KeepValues []string `mapstructure:"keep_values"`

	// ObfuscateSQLValues will specify a set of keys for which their values
	// will be passed through SQL obfuscation
	ObfuscateSQLValues []string `mapstructure:"obfuscate_sql_values"`
}
