package config

import "gopkg.in/yaml.v2"

// Provider is the interface any object gathering configuration
// options from various places has to implement
type Provider interface {
	Configure(*Config) error
}

// Config contains any possible configuration parameter for the Agent
type Config struct {
	// The host of the Datadog intake server to send Agent data to
	DdURL string `yaml:"dd_url"`
	// If you need a proxy to connect to the Internet, provide the settings here (default: disabled)
	ProxyHost string `yaml:"proxy_host"`
	ProxyPort int    `yaml:"proxy_port"`
	ProxyUser string `yaml:"proxy_user"`
	ProxyPass string `yaml:"proxy_password"`
	// To be used with some proxys that return a 302 which make curl switch from POST to GET
	// See http://stackoverflow.com/questions/8156073/curl-violate-rfc-2616-10-3-2-and-switch-from-post-to-get
	ProxyForbidMethodSwitch bool `yaml:"proxy_forbid_method_switch"`

	// If you run the agent behind haproxy, you might want to enable this
	SkipSSLValidation bool `yaml:"skip_ssl_validation"`

	// The Datadog api key to associate your Agent's data with your organization.
	// Can be found here: https://app.datadoghq.com/account/settings
	APIKey string `yaml:"api_key"`

	// Force the hostname to whatever you want. (default: auto-detected)
	HostName string `yaml:"hostname"`

	// Set the host's tags (optional)
	Tags string `yaml:"tags"`

	// Set timeout in seconds for outgoing requests to Datadog. (default: 20)
	// When a request timeout, it will be retried after some time.
	// It will only be deleted if the forwarder queue becomes too big. (30 MB by default)
	ForwarderTimeout int `yaml:"forwarder_timeout"`

	// Set timeout in seconds for integrations that use HTTP to fetch metrics, since
	// unbounded timeouts can potentially block the collector indefinitely and cause
	// problems!
	DefaultIntegrationHTTPTimeout int `yaml:"default_integration_http_timeout"`

	// Add one "dd_check:checkname" tag per running check. It makes it possible to slice
	// and dice per monitored app (= running Agent Check) on Datadog's backend.
	CreateDDCheckTags bool `yaml:"create_dd_check_tags"`

	// Collect AWS EC2 custom tags as agent tags (requires an IAM role associated with the instance)
	CollectEC2Tags bool `yaml:"collect_ec2_tags"`
	// Incorporate security-groups into tags collected from AWS EC2
	CollectSecurityGroups bool `yaml:"collect_security_groups"`

	// Enable Agent Developer Mode
	// Agent Developer Mode collects and sends more fine-grained metrics about agent and check performance
	DeveloperMode bool `yaml:"developer_mode"`
	// In developer mode, the number of runs to be included in a single collector profile
	CollectorProfileInterval bool `yaml:"collector_profile_interval"`

	// use unique hostname for GCE hosts, see http://dtdg.co/1eAynZk
	GCEUpdatedHostname bool `yaml:"gce_updated_hostname"`

	// Set the threshold for accepting points to allow anything
	// within recent_point_threshold seconds (default: 30)
	RecentPointThreshold int `yaml:"recent_point_threshold"`

	// Additional directory to look for Datadog checks (optional)
	AdditionalChecksd string `yaml:"additional_checksd"`

	// If enabled the collector will capture a metric for check run times.
	CheckTimings bool `yaml:"check_timings"`

	// If you want to remove the 'ww' flag from ps catching the arguments of processes
	// for instance for security reasons
	ExcludeProcessArgs bool `yaml:"exclude_process_args"`

	HistogramAggregates  string  `yaml:"histogram_aggregates"`
	HistogramPercentiles float64 `yaml:"histogram_percentiles"`

	// In some environments we may have the procfs file system mounted in a
	// miscellaneous location. The procfs_path configuration paramenter allows
	// us to override the standard default location '/proc'
	ProcfsPath string `yaml:"procfs_path"`

	LogLevel    string `yaml:"log_level"`
	LogFile     string `yaml:"collector_log_file"`
	LogToSyslog bool   `yaml:"log_to_syslog"`
	SyslogHost  string `yaml:"syslog_host"`
	SyslogPort  int    `yaml:"syslog_port"`
}

// NewConfig creates a new Config instance
func NewConfig() *Config {
	return &Config{
		DdURL:                "https://app.datadoghq.com",
		ForwarderTimeout:     20,
		RecentPointThreshold: 30,
		HistogramAggregates:  "max, median, avg, count",
		HistogramPercentiles: 0.95,
		ProcfsPath:           "/proc",
	}
}

// FromYAML tries to load the configration from an array of Bytes
// containing YAML code
func (c *Config) FromYAML(data []byte) error {
	err := yaml.Unmarshal(data, c)
	if err != nil {
		return err
	}

	return nil
}
