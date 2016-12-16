package dogstream

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// StreamEntry represents an instance in the config file
type StreamEntry struct {
	Name   string   `yaml:"name"`
	File   string   `yaml:"file"`
	Parser []string `yaml:"parser"`
}

type streamConfig struct {
	Instances []StreamEntry `yaml:"instances"`
}

func loadConfig(filename string) (map[string][]Parser, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var config streamConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	retmap := make(map[string][]Parser)

	for _, inst := range config.Instances {
		for _, parser := range inst.Parser {
			dsp, err := Load(parser)
			if err != nil {
				return nil, err
			}
			retmap[inst.File] = append(retmap[inst.File], dsp)
		}
	}
	return retmap, nil
}
