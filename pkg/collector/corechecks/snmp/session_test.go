package snmp

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	stdlog "log"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func Test_snmpSession_Configure(t *testing.T) {
	tests := []struct {
		name                       string
		config                     snmpConfig
		expectedError              error
		expectedVersion            gosnmp.SnmpVersion
		expectedTimeout            time.Duration
		expectedRetries            int
		expectedCommunity          string
		expectedMsgFlags           gosnmp.SnmpV3MsgFlags
		expectedContextName        string
		expectedSecurityParameters gosnmp.SnmpV3SecurityParameters
	}{
		{
			name: "no auth method",
			config: snmpConfig{
				ipAddress: "1.2.3.4",
				port:      uint16(1234),
			},
			expectedError: fmt.Errorf("an authentication method needs to be provided"),
		},
		{
			name: "valid v1 config",
			config: snmpConfig{
				ipAddress:       "1.2.3.4",
				port:            uint16(1234),
				snmpVersion:     "1",
				timeout:         4,
				retries:         3,
				communityString: "abc",
			},
			expectedVersion:   gosnmp.Version1,
			expectedError:     nil,
			expectedTimeout:   time.Duration(4) * time.Second,
			expectedRetries:   3,
			expectedCommunity: "abc",
			expectedMsgFlags:  gosnmp.NoAuthNoPriv,
		},
		{
			name: "valid default v2 config",
			config: snmpConfig{
				ipAddress:       "1.2.3.4",
				port:            uint16(1234),
				timeout:         4,
				retries:         3,
				communityString: "abc",
			},
			expectedVersion:   gosnmp.Version2c,
			expectedError:     nil,
			expectedTimeout:   time.Duration(4) * time.Second,
			expectedRetries:   3,
			expectedCommunity: "abc",
			expectedMsgFlags:  gosnmp.NoAuthNoPriv,
		},
		{
			name: "valid v2 config",
			config: snmpConfig{
				ipAddress:       "1.2.3.4",
				port:            uint16(1234),
				timeout:         4,
				retries:         3,
				communityString: "abc",
			},
			expectedVersion:   gosnmp.Version2c,
			expectedError:     nil,
			expectedTimeout:   time.Duration(4) * time.Second,
			expectedRetries:   3,
			expectedCommunity: "abc",
			expectedMsgFlags:  gosnmp.NoAuthNoPriv,
		},
		{
			name: "valid v2c config",
			config: snmpConfig{
				ipAddress:       "1.2.3.4",
				port:            uint16(1234),
				timeout:         4,
				retries:         3,
				communityString: "abc",
			},
			expectedVersion:   gosnmp.Version2c,
			expectedError:     nil,
			expectedTimeout:   time.Duration(4) * time.Second,
			expectedRetries:   3,
			expectedCommunity: "abc",
			expectedMsgFlags:  gosnmp.NoAuthNoPriv,
		},
		{
			name: "valid v3 AuthPriv config",
			config: snmpConfig{
				ipAddress:    "1.2.3.4",
				port:         uint16(1234),
				timeout:      4,
				retries:      3,
				contextName:  "myContext",
				user:         "myUser",
				authKey:      "myAuthKey",
				authProtocol: "md5",
				privKey:      "myPrivKey",
				privProtocol: "aes",
			},
			expectedVersion:     gosnmp.Version3,
			expectedError:       nil,
			expectedTimeout:     time.Duration(4) * time.Second,
			expectedRetries:     3,
			expectedCommunity:   "",
			expectedMsgFlags:    gosnmp.AuthPriv,
			expectedContextName: "myContext",
			expectedSecurityParameters: &gosnmp.UsmSecurityParameters{
				UserName:                 "myUser",
				AuthenticationProtocol:   gosnmp.MD5,
				AuthenticationPassphrase: "myAuthKey",
				PrivacyProtocol:          gosnmp.AES,
				PrivacyPassphrase:        "myPrivKey",
			},
		},
		{
			name: "valid v3 AuthNoPriv config",
			config: snmpConfig{
				ipAddress:    "1.2.3.4",
				port:         uint16(1234),
				timeout:      4,
				retries:      3,
				user:         "myUser",
				authKey:      "myAuthKey",
				authProtocol: "md5",
			},
			expectedVersion:   gosnmp.Version3,
			expectedError:     nil,
			expectedTimeout:   time.Duration(4) * time.Second,
			expectedRetries:   3,
			expectedCommunity: "",
			expectedMsgFlags:  gosnmp.AuthNoPriv,
			expectedSecurityParameters: &gosnmp.UsmSecurityParameters{
				UserName:                 "myUser",
				AuthenticationProtocol:   gosnmp.MD5,
				AuthenticationPassphrase: "myAuthKey",
				PrivacyProtocol:          gosnmp.NoPriv,
				PrivacyPassphrase:        "",
			},
		},
		{
			name: "invalid v3 authProtocol",
			config: snmpConfig{
				ipAddress:    "1.2.3.4",
				port:         uint16(1234),
				timeout:      4,
				retries:      3,
				user:         "myUser",
				authKey:      "myAuthKey",
				authProtocol: "invalid",
			},
			expectedVersion:            gosnmp.Version1, // default, not configured
			expectedError:              fmt.Errorf("unsupported authentication protocol: invalid"),
			expectedSecurityParameters: nil, // default, not configured
		},
		{
			name: "invalid v3 privProtocol",
			config: snmpConfig{
				ipAddress:    "1.2.3.4",
				port:         uint16(1234),
				timeout:      4,
				retries:      3,
				user:         "myUser",
				authKey:      "myAuthKey",
				authProtocol: "md5",
				privKey:      "myPrivKey",
				privProtocol: "invalid",
			},
			expectedVersion:            gosnmp.Version1, // default, not configured
			expectedError:              fmt.Errorf("unsupported privacy protocol: invalid"),
			expectedSecurityParameters: nil, // default, not configured
		},
		{
			name: "batch size too big",
			config: snmpConfig{
				ipAddress:       "1.2.3.4",
				port:            uint16(1234),
				timeout:         4,
				retries:         3,
				communityString: "abc",
				oidBatchSize:    100,
			},
			expectedVersion: gosnmp.Version1,
			expectedError:   fmt.Errorf("config oidBatchSize (100) cannot be higher than gosnmp.MaxOids: 60"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &snmpSession{}
			err := s.Configure(tt.config)
			assert.Equal(t, tt.expectedError, err)
			assert.Equal(t, tt.expectedVersion, s.gosnmpInst.Version)
			assert.Equal(t, tt.expectedRetries, s.gosnmpInst.Retries)
			assert.Equal(t, tt.expectedTimeout, s.gosnmpInst.Timeout)
			assert.Equal(t, tt.expectedCommunity, s.gosnmpInst.Community)
			assert.Equal(t, tt.expectedContextName, s.gosnmpInst.ContextName)
			assert.Equal(t, tt.expectedMsgFlags, s.gosnmpInst.MsgFlags)
			assert.Equal(t, tt.expectedSecurityParameters, s.gosnmpInst.SecurityParameters)
		})
	}
}

func Test_snmpSession_traceLog_disabled(t *testing.T) {

	config := snmpConfig{
		ipAddress:       "1.2.3.4",
		communityString: "abc",
	}
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.InfoLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "info")

	s := &snmpSession{}
	err = s.Configure(config)
	assert.Nil(t, err)
	assert.Equal(t, false, s.loggerEnabled)
	assert.Nil(t, s.gosnmpInst.Logger)

}
func Test_snmpSession_traceLog_enabled(t *testing.T) {
	config := snmpConfig{
		ipAddress:       "1.2.3.4",
		communityString: "abc",
	}
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "trace")

	s := &snmpSession{}
	err = s.Configure(config)
	assert.Nil(t, err)
	assert.Equal(t, true, s.loggerEnabled)
	assert.NotNil(t, s.gosnmpInst.Logger)

	s.gosnmpInst.Logger.Print("log line 1")
	s.gosnmpInst.Logger.Print("log line 2")

	w.Flush()
	logs := b.String()

	assert.Contains(t, logs, "log line 1")
	assert.Contains(t, logs, "log line 2")

}

func Test_snmpSession_Connect_Logger(t *testing.T) {
	config := snmpConfig{
		ipAddress:       "1.2.3.4",
		communityString: "abc",
	}
	s := &snmpSession{}
	err := s.Configure(config)
	require.NoError(t, err)

	logger := stdlog.New(ioutil.Discard, "abc", 0)
	s.loggerEnabled = false
	s.gosnmpInst.Logger = logger
	s.Connect()
	assert.NotSame(t, logger, s.gosnmpInst.Logger)

	s.loggerEnabled = true
	s.gosnmpInst.Logger = logger
	s.Connect()
	assert.Same(t, logger, s.gosnmpInst.Logger)
}
