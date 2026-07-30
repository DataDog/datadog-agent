// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

var (
	initialMetadataURL = ec2internal.MetadataURL
	initialTokenURL    = ec2internal.TokenURL
)

const testIMDSToken = "AQAAAFKw7LyqwVmmBMkqXHpDBuDWw2GnfGswTHi2yiIOGvzD7OMaWw=="

func resetPackageVars() {
	ec2internal.MetadataURL = initialMetadataURL
	ec2internal.TokenURL = initialTokenURL
	ec2internal.Token = httputils.NewAPIToken(ec2internal.GetToken)
	ec2internal.CurrentMetadataSource = ec2internal.MetadataSourceNone

	instanceIDFetcher.Reset()
	publicIPv4Fetcher.Reset()
	hostnameFetcher.Reset()
	networkIDFetcher.Reset()
}

func setupDMIForEC2(t *testing.T) {
	dmi.SetupMock(t, "ec2something", "ec2something2", "i-myinstance", DMIBoardVendor)
}

func setupDMIForNotEC2(t *testing.T) {
	dmi.SetupMock(t, "", "", "", "")
}

func TestIsDefaultHostname(t *testing.T) {
	conf := configmock.New(t)

	for _, prefix := range []bool{true, false} {
		conf.SetDefault("ec2_use_windows_prefix_detection", prefix)

		assert.True(t, IsDefaultHostname("IP-FOO"))
		assert.True(t, IsDefaultHostname("domuarigato"))
		assert.Equal(t, prefix, IsDefaultHostname("EC2AMAZ-FOO"))
		assert.False(t, IsDefaultHostname(""))
	}
}

func TestIsDefaultHostnameForIntake(t *testing.T) {
	conf := configmock.New(t)
	conf.SetDefault("ec2_use_windows_prefix_detection", true)

	assert.True(t, IsDefaultHostnameForIntake("IP-FOO"))
	assert.True(t, IsDefaultHostnameForIntake("domuarigato"))
	assert.False(t, IsDefaultHostnameForIntake("EC2AMAZ-FOO"))
	assert.True(t, IsDefaultHostname("EC2AMAZ-FOO"))
}

func TestGetInstanceID(t *testing.T) {
	ctx := context.Background()
	var expected string
	var responseCode int
	var lastRequest *http.Request

	// Force refresh
	ec2internal.Token.ExpirationDate = time.Now()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut:
			// Should be a token request
			io.WriteString(w, testIMDSToken)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			// Should be a metadata request
			t := r.Header.Get("X-aws-ec2-metadata-token")
			if t != testIMDSToken {
				w.WriteHeader(http.StatusUnauthorized)
			}
			io.WriteString(w, expected)
			w.WriteHeader(responseCode)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		lastRequest = r
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_metadata_timeout", 1000)

	// Ensure failures if we fail to use the local mock metadata server
	setupDMIForNotEC2(t)
	conf.SetInTest("ec2_use_dmi", true)

	// Ensure that the local server is up before checking values
	assert.EventuallyWithT(
		t,
		func(c *assert.CollectT) {
			resp, err := http.Get(ts.URL)
			require.NoError(c, err)
			assert.Equal(c, http.StatusUnauthorized, resp.StatusCode)
			resp.Body.Close()
		},
		time.Second,
		10*time.Millisecond,
		"Mock AWS metadata server was unable to be reached (%v)",
		ts.URL,
	)

	// API successful, should return API result
	responseCode = http.StatusOK
	expected = "i-0123456789abcdef0"
	val, err := GetInstanceID(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// the internal cache is populated now, should return the cached value even if API errors out
	responseCode = http.StatusInternalServerError
	val, err = GetInstanceID(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// the internal cache is populated, should refresh result if API call succeeds
	responseCode = http.StatusOK
	expected = "i-aaaaaaaaaaaaaaaaa"
	val, err = GetInstanceID(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")
}

func TestGetLegacyResolutionInstanceID(t *testing.T) {
	ctx := context.Background()
	expected := "i-0123456789abcdef0"
	var responseCode int
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(responseCode)
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_metadata_timeout", 1000)

	// API errors out, should return error
	responseCode = http.StatusInternalServerError
	val, err := GetLegacyResolutionInstanceID(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, "", val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// API successful, should return API result
	responseCode = http.StatusOK
	val, err = GetLegacyResolutionInstanceID(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// the internal cache is populated now, should return the cached value even if API errors out
	responseCode = http.StatusInternalServerError
	val, err = GetLegacyResolutionInstanceID(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")

	// the internal cache is populated, should refresh result if API call succeeds
	responseCode = http.StatusOK
	expected = "i-aaaaaaaaaaaaaaaaa"
	val, err = GetLegacyResolutionInstanceID(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/instance-id")
}

func TestGetHostAliases(t *testing.T) {
	conf := configmock.New(t)
	tests := []struct {
		name          string
		instanceID    string
		expectedHosts []string
		setupDMI      bool
		disableDMI    bool
	}{
		{
			name:          "Instance ID found",
			instanceID:    "i-0b22a22eec53b9321",
			expectedHosts: []string{"i-0b22a22eec53b9321"},
			setupDMI:      false,
		},
		{
			name:          "No Instance ID found",
			expectedHosts: []string{},
			setupDMI:      false,
		},
		{
			name:          "Instance ID found with DMI",
			expectedHosts: []string{"i-myinstance"},
			setupDMI:      true,
		},
		{
			name:          "Instance ID found with DMI",
			expectedHosts: []string{},
			setupDMI:      true,
			disableDMI:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupDMI {
				setupDMIForEC2(t)
			} else {
				setupDMIForNotEC2(t)
			}

			conf.SetInTest("ec2_use_dmi", !tc.disableDMI)

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				var responseCode int
				if tc.instanceID != "" {
					responseCode = http.StatusOK
				} else {
					responseCode = http.StatusInternalServerError
				}
				w.WriteHeader(responseCode)
				_, _ = io.WriteString(w, tc.instanceID)
			}))
			defer ts.Close()
			defer resetPackageVars()

			ec2internal.MetadataURL = ts.URL
			conf.SetInTest("ec2_metadata_timeout", 1000)

			ctx := context.Background()
			aliases, err := GetHostAliases(ctx)
			assert.Equal(t, tc.expectedHosts, aliases)
			assert.NoError(t, err)
		})
	}
}

func TestGetHostname(t *testing.T) {
	ctx := context.Background()
	expected := "ip-10-10-10-10.ec2.internal"
	var responseCode int
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// save the last request before writing the response to avoid a race when asserting
		lastRequest = r

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(responseCode)
		io.WriteString(w, expected)
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL

	conf := configmock.New(t)
	defer resetPackageVars()

	conf.SetInTest("ec2_metadata_timeout", 1000)

	// API errors out, should return error
	responseCode = http.StatusInternalServerError
	val, err := GetHostname(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, "", val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// API successful, should return hostname
	responseCode = http.StatusOK
	val, err = GetHostname(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// the internal cache is populated now, should return the cached hostname even if API errors out
	responseCode = http.StatusInternalServerError
	val, err = GetHostname(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// the internal cache is populated, should refresh result if API call succeeds
	responseCode = http.StatusOK
	expected = "ip-20-20-20-20.ec2.internal"
	val, err = GetHostname(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")

	// clear internal cache
	hostnameFetcher.Reset()

	// ensure we get an empty string along with the error when not on EC2
	ec2internal.MetadataURL = "foo"
	val, err = GetHostname(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, "", val)
	assert.Equal(t, lastRequest.URL.Path, "/hostname")
}

func TestGetToken(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		h := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
		if h != "" && r.Method == http.MethodPut {
			io.WriteString(w, testIMDSToken)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	conf := configmock.New(t)
	defer resetPackageVars()

	defer ts.Close()
	ec2internal.TokenURL = ts.URL
	conf.SetInTest("ec2_metadata_timeout", 1000)

	token, err := ec2internal.Token.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, testIMDSToken, token)
}

func TestMetedataRequestWithToken(t *testing.T) {
	conf := configmock.New(t)
	testCases := []struct {
		name        string
		configKey   string
		configValue bool
	}{
		{
			name:        "IMDSv2 Preferred",
			configKey:   "ec2_prefer_imdsv2",
			configValue: true,
		},
		{
			name:        "IMDSv2 Transition Payload Enabled",
			configKey:   "ec2_imdsv2_transition_payload_enabled",
			configValue: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var requestWithoutToken *http.Request
			var requestForToken *http.Request
			var requestWithToken *http.Request
			var seq int
			ctx := context.Background()

			ipv4 := "198.51.100.1"

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				switch r.Method {
				case http.MethodPut:
					// Should be a token request
					h := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
					if h == "" {
						w.WriteHeader(http.StatusUnauthorized)
					}
					r.Header.Add("X-sequence", strconv.Itoa(seq))
					seq++
					requestForToken = r
					io.WriteString(w, testIMDSToken)
				case http.MethodGet:
					// Should be a metadata request
					t := r.Header.Get("X-aws-ec2-metadata-token")
					if t != testIMDSToken {
						r.Header.Add("X-sequence", strconv.Itoa(seq))
						seq++
						requestWithoutToken = r
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					switch r.RequestURI {
					case "/public-ipv4":
						r.Header.Add("X-sequence", strconv.Itoa(seq))
						seq++
						requestWithToken = r
						io.WriteString(w, ipv4)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer ts.Close()
			ec2internal.MetadataURL = ts.URL
			ec2internal.TokenURL = ts.URL

			// Set test-specific configuration
			defer resetPackageVars()
			conf.SetDefault(tc.configKey, tc.configValue)
			conf.SetInTest("ec2_metadata_timeout", 1000)

			ips, err := GetPublicIPv4(ctx)
			require.NoError(t, err)
			assert.Equal(t, ipv4, ips)

			assert.Nil(t, requestWithoutToken)

			assert.Equal(t, "0", requestForToken.Header.Get("X-sequence"))
			assert.Equal(t, "1", requestWithToken.Header.Get("X-sequence"))
			assert.Equal(t, strconv.Itoa(conf.GetInt("ec2_metadata_token_lifetime")), requestForToken.Header.Get("X-aws-ec2-metadata-token-ttl-seconds"))
			assert.Equal(t, http.MethodPut, requestForToken.Method)
			assert.Equal(t, "/", requestForToken.RequestURI)
			assert.Equal(t, testIMDSToken, requestWithToken.Header.Get("X-aws-ec2-metadata-token"))
			assert.Equal(t, "/public-ipv4", requestWithToken.RequestURI)
			assert.Equal(t, http.MethodGet, requestWithToken.Method)

			// Ensure token has been cached
			ips, err = GetPublicIPv4(ctx)
			require.NoError(t, err)
			assert.Equal(t, ipv4, ips)
			// Unchanged
			assert.Equal(t, "0", requestForToken.Header.Get("X-sequence"))
			// Incremented
			assert.Equal(t, "2", requestWithToken.Header.Get("X-sequence"))

			// Force refresh
			ec2internal.Token.ExpirationDate = time.Now()
			ips, err = GetPublicIPv4(ctx)
			require.NoError(t, err)
			assert.Equal(t, ipv4, ips)
			// Incremented
			assert.Equal(t, "3", requestForToken.Header.Get("X-sequence"))
			assert.Equal(t, "4", requestWithToken.Header.Get("X-sequence"))
		})
	}
}

func TestLegacyMetedataRequestWithoutToken(t *testing.T) {
	var requestWithoutToken *http.Request
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetDefault("ec2_prefer_imdsv2", false)
	conf.SetDefault("ec2_imdsv2_transition_payload_enabled", false)

	ipv4 := "198.51.100.1"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Put is only use for token
		assert.NotEqual(t, r.Method, http.MethodPut)
		switch r.Method {
		case http.MethodGet:
			// Should be a metadata request without token
			token := r.Header.Get("X-aws-ec2-metadata-token")
			assert.Equal(t, token, "")
			switch r.RequestURI {
			case "/public-ipv4":
				requestWithoutToken = r
				io.WriteString(w, ipv4)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()
	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf.SetInTest("ec2_metadata_timeout", 1000)

	ips, err := GetPublicIPv4(context.Background())
	require.NoError(t, err)
	assert.Equal(t, ipv4, ips)

	assert.Equal(t, "/public-ipv4", requestWithoutToken.RequestURI)
	assert.Equal(t, http.MethodGet, requestWithoutToken.Method)
}

func TestGetNTPHostsFromIMDS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()
	configmock.New(t)
	defer resetPackageVars()

	ec2internal.MetadataURL = ts.URL
	actualHosts := GetNTPHosts(context.Background())
	assert.Equal(t, []string{"169.254.169.123"}, actualHosts)
}

func TestGetNTPHostsDMI(t *testing.T) {
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_use_dmi", true)

	setupDMIForEC2(t)
	ec2internal.MetadataURL = ""

	actualHosts := GetNTPHosts(context.Background())
	assert.Equal(t, []string{"169.254.169.123"}, actualHosts)
}

func TestGetNTPHostsEC2UUID(t *testing.T) {
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_use_dmi", true)

	dmi.SetupMock(t, "ec2something", "", "", "")
	ec2internal.MetadataURL = ""

	actualHosts := GetNTPHosts(context.Background())
	assert.Equal(t, []string{"169.254.169.123"}, actualHosts)
}

func TestGetNTPHostsDisabledDMI(t *testing.T) {
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_use_dmi", false)

	// DMI without EC2 UUID
	dmi.SetupMock(t, "something", "something", "i-myinstance", DMIBoardVendor)
	ec2internal.MetadataURL = ""

	actualHosts := GetNTPHosts(context.Background())
	assert.Equal(t, []string(nil), actualHosts)
}

func TestGetNTPHostsNotEC2(t *testing.T) {
	setupDMIForNotEC2(t)
	ec2internal.MetadataURL = ""

	actualHosts := GetNTPHosts(context.Background())
	assert.Equal(t, []string(nil), actualHosts)
}

func TestMetadataSourceIMDS(t *testing.T) {
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.Method {
		case http.MethodPut: // token request
			io.WriteString(w, testIMDSToken)
		case http.MethodGet: // metadata request
			switch r.RequestURI {
			case "/hostname":
				io.WriteString(w, "ip-10-10-10-10.ec2.internal")
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	ec2internal.MetadataURL = ts.URL
	ec2internal.TokenURL = ts.URL
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_metadata_timeout", 1000)
	conf.SetInTest("ec2_prefer_imdsv2", true)
	conf.SetInTest("ec2_imdsv2_transition_payload_enabled", false)

	assert.True(t, IsRunningOn(ctx))
	assert.Equal(t, ec2internal.MetadataSourceIMDSv2, ec2internal.CurrentMetadataSource)

	hostnameFetcher.Reset()
	ec2internal.CurrentMetadataSource = ec2internal.MetadataSourceNone
	conf.SetInTest("ec2_prefer_imdsv2", false)
	conf.SetInTest("ec2_imdsv2_transition_payload_enabled", true)
	assert.True(t, IsRunningOn(ctx))
	assert.Equal(t, ec2internal.MetadataSourceIMDSv2, ec2internal.CurrentMetadataSource)

	// trying IMDSv1
	hostnameFetcher.Reset()
	ec2internal.CurrentMetadataSource = ec2internal.MetadataSourceNone
	conf.SetInTest("ec2_prefer_imdsv2", false)
	conf.SetInTest("ec2_imdsv2_transition_payload_enabled", false)

	assert.True(t, IsRunningOn(ctx))
	assert.Equal(t, ec2internal.MetadataSourceIMDSv1, ec2internal.CurrentMetadataSource)
}

func TestMetadataSourceUUID(t *testing.T) {
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_use_dmi", true)

	ctx := context.Background()

	ec2internal.MetadataURL = ""

	dmi.SetupMock(t, "ec2something", "", "", "")
	assert.True(t, IsRunningOn(ctx))
	assert.Equal(t, ec2internal.MetadataSourceUUID, ec2internal.CurrentMetadataSource)

	dmi.SetupMock(t, "", "ec2something", "", "")
	assert.True(t, IsRunningOn(ctx))
	assert.Equal(t, ec2internal.MetadataSourceUUID, ec2internal.CurrentMetadataSource)

	dmi.SetupMock(t, "", "45E12AEC-DCD1-B213-94ED-012345ABCDEF", "", "")
	assert.True(t, IsRunningOn(ctx))
	assert.Equal(t, ec2internal.MetadataSourceUUID, ec2internal.CurrentMetadataSource)
}

func TestMetadataSourceDMI(t *testing.T) {
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_use_dmi", true)

	ctx := context.Background()

	ec2internal.MetadataURL = ""

	setupDMIForEC2(t)
	GetHostAliases(ctx)
	assert.Equal(t, ec2internal.MetadataSourceDMI, ec2internal.CurrentMetadataSource)
}

func TestMetadataSourceDMIPreventFallback(t *testing.T) {
	conf := configmock.New(t)
	defer resetPackageVars()
	conf.SetInTest("ec2_use_dmi", true)

	ctx := context.Background()

	ec2internal.MetadataURL = ""

	setupDMIForEC2(t)
	GetHostAliases(ctx)
	assert.Equal(t, ec2internal.MetadataSourceDMI, ec2internal.CurrentMetadataSource)

	assert.True(t, IsRunningOn(ctx))
	assert.Equal(t, ec2internal.MetadataSourceDMI, ec2internal.CurrentMetadataSource)
}
