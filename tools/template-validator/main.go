package main

import (
	"fmt"
	"os"
	"strings"
)

type ConfigTemplateAnalyzer struct {
	currSetting *setting
	settingList []setting
	settingMap  map[string]int
}

type setting struct {
	name string
	metadata []string
	define []string
	// TODO: name, env var, type, default, docs
}

func newConfigTemplateAnalyzer() *ConfigTemplateAnalyzer {
	return &ConfigTemplateAnalyzer{
		settingMap: make(map[string]int),
	}
}

func (c *ConfigTemplateAnalyzer) Run(content string) error {
	isInSectionHeader := false
	for _, line := range strings.Split(content, "\n") {
		// TODO: keep track of indentation
		line = strings.TrimSpace(line)

		if len(line) == 0 {
			c.FlushElement()
			isInSectionHeader = false
			continue
		}
		if strings.HasPrefix(line, "######") {
			isInSectionHeader = true
			continue
		}
		if isInSectionHeader && strings.HasPrefix(line, "## ") {
			// TODO: section name
			continue
		}

		if strings.HasPrefix(line, "## ") {
			c.AddMetadata(strings.TrimPrefix(line, "## "))
			continue
		}
		if strings.HasPrefix(line, "# ") {
			c.AddDefine(strings.TrimPrefix(line, "# "))
			continue
		}
		if line == "api_key:" {
			c.AddDefine(line)
			continue
		}

		// ignore
		fmt.Printf("(ignoring %q)\n", line)
	}
	return nil
}

func (c *ConfigTemplateAnalyzer) AddMetadata(data string) {
	if c.currSetting == nil {
		c.currSetting = &setting{}
	}
	c.currSetting.metadata = append(c.currSetting.metadata, data)
}

func (c *ConfigTemplateAnalyzer) AddDefine(data string) {
	if c.currSetting == nil {
		c.currSetting = &setting{}
	}
	if len(c.currSetting.define) == 0 {
		c.currSetting.name = strings.Split(data, ":")[0]
	}
	c.currSetting.define = append(c.currSetting.define, data)
}

func (c *ConfigTemplateAnalyzer) FlushElement() {
	if c.currSetting == nil {
		return
	}
	index := len(c.settingList)
	c.settingList = append(c.settingList, *c.currSetting)
	c.settingMap[c.currSetting.name] = index
	c.currSetting = nil
}

func (c *ConfigTemplateAnalyzer) Dump() {
	fmt.Printf("number of settings: %d\n", len(c.settingList))
	for i := range 10 {
		fmt.Printf("------\n- %d: %v\n", i, c.settingList[i])
	}
}

func analyzeConfigTemplate() error {
	templateFilename := "pkg/config/config_template.yaml"
	content, err := os.ReadFile(templateFilename)
	if err != nil {
		return err
	}
	analyzer := newConfigTemplateAnalyzer()
	if err := analyzer.Run(string(content)); err != nil {
		return err
	}
	analyzer.Dump()
	return nil
}

func main() {
	if err := analyzeConfigTemplate(); err != nil {
		panic(err)
	}
}