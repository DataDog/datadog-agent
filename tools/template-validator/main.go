package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type ConfigTemplateAnalyzer struct {
	currSetting *setting
	settingList []setting
	settingMap  map[string]int
}

type setting struct {
	name   string
	define []string
	env    []envDecl
	param  paramDecl
	docs   []string
}

type envDecl struct {
	name       string
	typ        string
	defaultVal string
	required   string
}

type paramDecl struct {
	name       string
	typ        string
	defaultVal string
	required   string
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

func (c *ConfigTemplateAnalyzer) validateType(data string) error {
	knownTypes := []string{
		"string", "boolean", "integer", "duration", "float",
		"number", "int", "bool",
		"list", "json", "map", "custom", "object",
		"custom object",
		"list of strings",
		"list of objects",
		"map of strings",
		"list of custom object",
		"list of custom objects",
		"list of key:value elements",
		"List of custom object",
		"list of comma separated strings",
	}
	if slices.Contains(knownTypes, data) {
		return nil
	}
	return fmt.Errorf("unknown type: %q", data)
}

func (c *ConfigTemplateAnalyzer) parseParamDecl(data string) paramDecl {
	res := paramDecl{}
	parts := strings.Split(data, " - ")
	index := 0

	for _, part := range parts {
		// TODO: validate each part
		part = strings.Trim(part, " ")
		switch index {
		case 0:
			res.name = part
			index += 1
		case 1:
			err := c.validateType(part)
			if err == nil {
				res.typ = part
				index += 1
			} else {
				fmt.Printf("[ERORR] config setting %s: %s\n", res.name, err)
				return res
			}
		case 2:
			res.required = part
			index += 2
		case 3:
			res.defaultVal = part
			index += 1
		}
	}

	return res
}

func (c *ConfigTemplateAnalyzer) parseEnvDecl(data string) envDecl {
	res := envDecl{}
	parts := strings.Split(data, " - ")
	index := 0

	for _, part := range parts {
		// TODO: validate each part
		part = strings.Trim(part, " ")
		switch index {
		case 0:
			res.name = part
			index += 1
		case 1:
			res.typ = part
			index += 1
		case 2:
			res.required = part
			index += 2
		case 3:
			res.defaultVal = part
			index += 1
		}
	}
	return res
}

// AddMetadata adds metadata that appears above a setting. It defines the name and type
// Example:
// ## @param min_tls_version - string - optional - default: "tlsv1.2"
// ## @env DD_MIN_TLS_VERSION - string - optional - default: "tlsv1.2"
func (c *ConfigTemplateAnalyzer) AddMetadata(data string) {
	if c.currSetting == nil {
		c.currSetting = &setting{}
	}
	if remain, has := strings.CutPrefix(data, "@param"); has {
		info := c.parseParamDecl(remain)
		c.currSetting.param = info
	} else if remain, has := strings.CutPrefix(data, "@env"); has {
		info := c.parseEnvDecl(remain)
		c.currSetting.env = append(c.currSetting.env, info)
	} else {
		c.currSetting.docs = append(c.currSetting.docs, data)
	}
	//	c.currSetting.metadata = append(c.currSetting.metadata, data)
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
		fmt.Printf("------\n")
		st := c.settingList[i]

		/*
			name   string
			define []string
			env    []envDecl
			param  paramDecl
			docs   []string
		*/

		fmt.Printf("- %d: name:%s param:{name:%s typ:%s def:%s req:%s} env:%v defs:%v docs:%v\n",
			i, st.name, st.param.name, st.param.typ, st.param.defaultVal, st.param.required,
			st.env, st.define, st.docs)
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
