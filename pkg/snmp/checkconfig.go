package snmp

// CheckConfig represents snmp integration config
type CheckConfig struct {
	IPAddress             string   `yaml:"ip_address"`
	Port                  uint16   `yaml:"port,omitempty"`
	CommunityString       string   `yaml:"community_string,omitempty"`
	SnmpVersion           string   `yaml:"snmp_version,omitempty"`
	Timeout               int      `yaml:"timeout,omitempty"`
	Retries               int      `yaml:"retries,omitempty"`
	OidBatchSize          int      `yaml:"oid_batch_size,omitempty"`
	User                  string   `yaml:"user,omitempty"`
	AuthProtocol          string   `yaml:"authProtocol,omitempty"`
	AuthKey               string   `yaml:"authKey,omitempty"`
	PrivProtocol          string   `yaml:"privProtocol,omitempty"`
	PrivKey               string   `yaml:"privKey,omitempty"`
	Tags                  []string `yaml:"tags,omitempty"`
	MinCollectionInterval *uint    `yaml:"min_collection_interval,omitempty"`
}
