// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package session

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	stdlog "log"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
)

func Test_snmpSession_Configure(t *testing.T) {
	tests := []struct {
		name                       string
		config                     checkconfig.CheckConfig
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
			config: checkconfig.CheckConfig{
				IPAddress: "1.2.3.4",
				Port:      uint16(1234),
			},
			expectedError: fmt.Errorf("an authentication method needs to be provided"),
		},
		{
			name: "valid v1 config",
			config: checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				Port:            uint16(1234),
				SnmpVersion:     "1",
				Timeout:         4,
				Retries:         3,
				CommunityString: "abc",
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
			config: checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				Port:            uint16(1234),
				Timeout:         4,
				Retries:         3,
				CommunityString: "abc",
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
			config: checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				Port:            uint16(1234),
				Timeout:         4,
				Retries:         3,
				CommunityString: "abc",
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
			config: checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				Port:            uint16(1234),
				Timeout:         4,
				Retries:         3,
				CommunityString: "abc",
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
			config: checkconfig.CheckConfig{
				IPAddress:    "1.2.3.4",
				Port:         uint16(1234),
				Timeout:      4,
				Retries:      3,
				ContextName:  "myContext",
				User:         "myUser",
				AuthKey:      "myAuthKey",
				AuthProtocol: "md5",
				PrivKey:      "myPrivKey",
				PrivProtocol: "aes",
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
			config: checkconfig.CheckConfig{
				IPAddress:    "1.2.3.4",
				Port:         uint16(1234),
				Timeout:      4,
				Retries:      3,
				User:         "myUser",
				AuthKey:      "myAuthKey",
				AuthProtocol: "md5",
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
			config: checkconfig.CheckConfig{
				IPAddress:    "1.2.3.4",
				Port:         uint16(1234),
				Timeout:      4,
				Retries:      3,
				User:         "myUser",
				AuthKey:      "myAuthKey",
				AuthProtocol: "invalid",
			},
			expectedVersion:            gosnmp.Version1, // default, not configured
			expectedError:              fmt.Errorf("unsupported authentication protocol: invalid"),
			expectedSecurityParameters: nil, // default, not configured
		},
		{
			name: "invalid v3 privProtocol",
			config: checkconfig.CheckConfig{
				IPAddress:    "1.2.3.4",
				Port:         uint16(1234),
				Timeout:      4,
				Retries:      3,
				User:         "myUser",
				AuthKey:      "myAuthKey",
				AuthProtocol: "md5",
				PrivKey:      "myPrivKey",
				PrivProtocol: "invalid",
			},
			expectedVersion:            gosnmp.Version1, // default, not configured
			expectedError:              fmt.Errorf("unsupported privacy protocol: invalid"),
			expectedSecurityParameters: nil, // default, not configured
		},
		{
			name: "batch size too big",
			config: checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				Port:            uint16(1234),
				Timeout:         4,
				Retries:         3,
				CommunityString: "abc",
				OidBatchSize:    100,
			},
			expectedVersion: gosnmp.Version1,
			expectedError:   fmt.Errorf("config oidBatchSize (100) cannot be higher than gosnmp.MaxOids: 60"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewGosnmpSession(&tt.config)
			assert.Equal(t, tt.expectedError, err)
			if tt.expectedError == nil {
				gosnmpSess := s.(*GosnmpSession)
				assert.Equal(t, tt.expectedVersion, gosnmpSess.gosnmpInst.Version)
				assert.Equal(t, tt.expectedRetries, gosnmpSess.gosnmpInst.Retries)
				assert.Equal(t, tt.expectedTimeout, gosnmpSess.gosnmpInst.Timeout)
				assert.Equal(t, tt.expectedCommunity, gosnmpSess.gosnmpInst.Community)
				assert.Equal(t, tt.expectedContextName, gosnmpSess.gosnmpInst.ContextName)
				assert.Equal(t, tt.expectedMsgFlags, gosnmpSess.gosnmpInst.MsgFlags)
				assert.Equal(t, tt.expectedSecurityParameters, gosnmpSess.gosnmpInst.SecurityParameters)
			}
		})
	}
}

func Test_snmpSession_traceLog_disabled(t *testing.T) {

	config := checkconfig.CheckConfig{
		IPAddress:       "1.2.3.4",
		CommunityString: "abc",
	}
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.InfoLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "info")

	s, err := NewGosnmpSession(&config)
	gosnmpSess := s.(*GosnmpSession)
	assert.Nil(t, err)
	assert.Equal(t, gosnmp.Logger{}, gosnmpSess.gosnmpInst.Logger)

}
func Test_snmpSession_traceLog_enabled(t *testing.T) {
	config := checkconfig.CheckConfig{
		IPAddress:       "1.2.3.4",
		CommunityString: "abc",
	}
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "trace")

	s, err := NewGosnmpSession(&config)
	gosnmpSess := s.(*GosnmpSession)
	assert.Nil(t, err)
	assert.NotNil(t, gosnmpSess.gosnmpInst.Logger)

	gosnmpSess.gosnmpInst.Logger.Print("log line 1")
	gosnmpSess.gosnmpInst.Logger.Print("log line 2")

	w.Flush()
	logs := b.String()

	assert.Contains(t, logs, "log line 1")
	assert.Contains(t, logs, "log line 2")

}

func Test_snmpSession_Connect_Logger(t *testing.T) {
	config := checkconfig.CheckConfig{
		IPAddress:       "1.2.3.4",
		CommunityString: "abc",
	}
	s, err := NewGosnmpSession(&config)
	gosnmpSess := s.(*GosnmpSession)
	require.NoError(t, err)

	logger := gosnmp.NewLogger(stdlog.New(io.Discard, "abc", 0))
	gosnmpSess.gosnmpInst.Logger = logger
	s.Connect()
	assert.Equal(t, logger, gosnmpSess.gosnmpInst.Logger)

	s.Connect()
	assert.Equal(t, logger, gosnmpSess.gosnmpInst.Logger)

	logger2 := gosnmp.NewLogger(stdlog.New(io.Discard, "123", 0))
	gosnmpSess.gosnmpInst.Logger = logger2
	s.Connect()
	assert.NotEqual(t, logger, gosnmpSess.gosnmpInst.Logger)
	assert.Equal(t, logger2, gosnmpSess.gosnmpInst.Logger)
}

func TestFetchAllOIDsUsingGetNext(t *testing.T) {
	sess := CreateMockSession()

	sess.On("GetNext", []string{"1.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.1.0", gosnmp.OctetString, []byte(`123`)), nil)

	// Scalar OIDs
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.1.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.2.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.2.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.3.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.3.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.9.1.2.1", gosnmp.OctetString, []byte(`123`)), nil)

	// Table OIDs
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.9.1.3"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.9.1.3.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.9.1.4"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.9.1.4.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.9.1.5"}).Return(CreateGetNextPacket("999", gosnmp.EndOfMibView, nil), nil)

	resultOIDs := FetchAllOIDsUsingGetNext(sess)
	assert.Equal(t, []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.9.1.2.1",
		"1.3.6.1.2.1.1.9.1.3.1",
		"1.3.6.1.2.1.1.9.1.4.1",
	}, resultOIDs)
}

func TestFetchAllOIDsUsingGetNext_invalidColumnOid(t *testing.T) {
	sess := CreateMockSession()

	sess.On("GetNext", []string{"1.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.9.1.2.1", gosnmp.OctetString, []byte(`123`)), nil)
	// Table OIDs
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.9.1.3"}).Return(CreateGetNextPacket("1.2.3.4.5", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.2.3.4.5"}).Return(CreateGetNextPacket("1.2.3.4.6.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.2.3.4.6.0"}).Return(CreateGetNextPacket("999", gosnmp.EndOfMibView, nil), nil)

	resultOIDs := FetchAllOIDsUsingGetNext(sess)
	assert.Equal(t, []string{
		"1.3.6.1.2.1.1.9.1.2.1",
		"1.2.3.4.5",
		"1.2.3.4.6.0",
	}, resultOIDs)
}

func TestFetchAllOIDsUsingGetNext_handleNonSequentialOIDs(t *testing.T) {
	sess := CreateMockSession()

	sess.On("GetNext", []string{"1.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.9.1.2.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.9.1.3"}).Return(CreateGetNextPacket("1.2.3.4.5", gosnmp.OctetString, []byte(`123`)), nil)

	// invalid non-sequential oid that might lead to infinite loop if not handled
	sess.On("GetNext", []string{"1.2.3.4.5"}).Return(CreateGetNextPacket("1.2.3.4.5", gosnmp.OctetString, []byte(`123`)), nil)

	resultOIDs := FetchAllOIDsUsingGetNext(sess)
	assert.Equal(t, []string{
		"1.3.6.1.2.1.1.9.1.2.1",
		"1.2.3.4.5",
	}, resultOIDs)
}

func TestFetchAllOIDsUsingGetNext_End(t *testing.T) {
	tests := []struct {
		valueType gosnmp.Asn1BER
	}{
		{
			valueType: gosnmp.EndOfMibView,
		},
		{
			valueType: gosnmp.EndOfContents,
		},
	}
	for _, test := range tests {
		t.Run(test.valueType.String(), func(t *testing.T) {
			sess := CreateMockSession()
			sess.On("GetNext", []string{"1.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.1.0", gosnmp.OctetString, []byte(`123`)), nil)
			sess.On("GetNext", []string{"1.3.6.1.2.1.1.1.0"}).Return(CreateGetNextPacket("1.3.6.1.2.1.1.2.0", gosnmp.OctetString, []byte(`123`)), nil)
			sess.On("GetNext", []string{"1.3.6.1.2.1.1.2.0"}).Return(CreateGetNextPacket("", test.valueType, nil), nil)
			resultOIDs := FetchAllOIDsUsingGetNext(sess)
			assert.Equal(t, []string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.2.0"}, resultOIDs)

		})
	}
}

func TestFetchAllOIDsUsingGetNext_invalidMoreThanOneVariables(t *testing.T) {
	sess := CreateMockSession()

	packets := CreateGetNextPacket("1.3.6.1.2.1.1.9.1.2.1", gosnmp.OctetString, []byte(`123`))
	packets.Variables = append(packets.Variables, packets.Variables[0])
	sess.On("GetNext", []string{"1.0"}).Return(packets, nil)

	resultOIDs := FetchAllOIDsUsingGetNext(sess) // no packet created if variables != 1
	assert.Equal(t, []string(nil), resultOIDs)
}

func TestFetchAllOIDsUsingGetNext_invalidZeroVariable(t *testing.T) {
	sess := CreateMockSession()
	packets := &gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{},
	}
	sess.On("GetNext", []string{"1.0"}).Return(packets, nil)

	resultOIDs := FetchAllOIDsUsingGetNext(sess) // no packet created if variables != 1
	assert.Equal(t, []string(nil), resultOIDs)
}
