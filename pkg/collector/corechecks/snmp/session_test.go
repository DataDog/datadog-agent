package snmp

import (
	"fmt"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
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
			name: "invalid version",
			config: snmpConfig{
				ipAddress:       "1.2.3.4",
				port:            uint16(1234),
				snmpVersion:     "x",
				timeout:         4,
				retries:         3,
				communityString: "abc",
			},
			expectedVersion: gosnmp.Version1,
			expectedError:   fmt.Errorf("invalid snmp version `x`. Valid versions are: 1, 2, 2c, 3"),
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
				snmpVersion:     "",
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
				snmpVersion:     "2",
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
				snmpVersion:     "2c",
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
				snmpVersion:  "3",
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
				snmpVersion:  "3",
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
				snmpVersion:  "3",
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
				snmpVersion:  "3",
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
				snmpVersion:     "2",
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

func Test_snmpSession_GetBulk_emptyOids(t *testing.T) {
	s := &snmpSession{}
	packet, err := s.GetBulk([]string{})
	assert.Nil(t, err)
	assert.Equal(t, packet, &gosnmp.SnmpPacket{})
}
