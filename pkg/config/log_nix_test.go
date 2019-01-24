// +build linux freebsd netbsd openbsd solaris dragonfly darwin

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSyslogURI(t *testing.T) {
	assert := assert.New(t)

	mockConfig := Mock()
	mockConfig.Set("log_to_syslog", true)
	mockConfig.Set("syslog_uri", "")

	assert.Equal(GetSyslogURI(), defaultSyslogURI)

	mockConfig.Set("syslog_uri", "tcp://localhost:514")
	assert.Equal(GetSyslogURI(), "tcp://localhost:514")

	mockConfig.Set("log_to_syslog", false)
	assert.Equal(GetSyslogURI(), "")

	mockConfig.Set("syslog_uri", "")
	assert.Equal(GetSyslogURI(), "")
}
