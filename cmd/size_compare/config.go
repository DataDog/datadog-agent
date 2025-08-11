package main

type configImpl struct {
	data   map[string]interface{}
	paths  []string
	schema map[string]interface{}
}

type Config interface {
	Reader
}

type Reader interface {
	Get(key string) interface{}
	GetInt(key string) int
	GetBool(key string) bool
	GetString(key string) string
	AddConfigPath(path string)
}

type BuildableConfig interface {
	Reader
	DefineSetting(key string, defaultValue interface{})
}

func NewConfig() BuildableConfig {
	return &configImpl{
		data:   make(map[string]interface{}),
		schema: make(map[string]interface{}),
	}
}

func (c *configImpl) Get(key string) interface{} {
	return c.data[key]
}

func (c *configImpl) GetInt(key string) int {
	if num, ok := c.data[key].(int); ok {
		return num
	}
	return 0
}

func (c *configImpl) GetBool(key string) bool {
	if b, ok := c.data[key].(bool); ok {
		return b
	}
	return false
}

func (c *configImpl) GetString(key string) string {
	if str, ok := c.data[key].(string); ok {
		return str
	}
	return ""
}

func (c *configImpl) AddConfigPath(path string) {
	c.paths = append(c.paths, path)
}

func (c *configImpl) DefineSetting(key string, defaultValue interface{}) {
	c.schema[key] = defaultValue
	c.data[key] = defaultValue
}
