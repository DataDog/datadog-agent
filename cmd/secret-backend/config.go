package main

import (
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type SecretConfigurations struct {
	Configs map[string]map[string]string `yaml:"secrets"`
}

func NewSecretConfigurations(configFile *string) map[string]map[string]string {
	configYAML, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.WithField("configFile", *configFile).
			WithError(err).Fatal("failed to configuration file")
	}

	configs := &SecretConfigurations{}
	if err := yaml.Unmarshal(configYAML, configs); err != nil {
		log.WithField("configFile", *configFile).
			WithError(err).Fatal("failed to unmarshal configuration yaml")
	}

	return configs.Configs
}
