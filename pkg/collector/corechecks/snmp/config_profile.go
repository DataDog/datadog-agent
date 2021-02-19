package snmp

type profileConfigMap map[string]profileConfig

type profileConfig struct {
	DefinitionFile string `yaml:"definition_file"`
}
