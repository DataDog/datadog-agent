package snmp

type CheckConfig struct {
	IPAddress          string   `yaml:"ip_address"`
	Port               uint16     `yaml:"port"`
	CommunityString    string   `yaml:"community_string"`
	SnmpVersion        string   `yaml:"snmp_version"`
	Timeout            int     `yaml:"timeout"`
	Retries            int     `yaml:"retries"`
	OidBatchSize       int     `yaml:"oid_batch_size"`
	User               string   `yaml:"user"`
	AuthProtocol       string   `yaml:"authProtocol"`
	AuthKey            string   `yaml:"authKey"`
	PrivProtocol       string   `yaml:"privProtocol"`
	PrivKey            string   `yaml:"privKey"`
	Tags               []string `yaml:"tags"`
}
