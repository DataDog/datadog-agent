package writer

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	assert := assert.New(t)
	url, err := url.Parse("test_url")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("blank", func(t *testing.T) {
		client := newClient(&config.AgentConfig{}, false)
		transport := client.Transport.(*http.Transport)
		assert.False(transport.TLSClientConfig.InsecureSkipVerify)
		assert.Nil(transport.Proxy)
	})

	t.Run("no_proxy", func(t *testing.T) {
		client := newClient(&config.AgentConfig{
			SkipSSLValidation: true,
			ProxyURL:          url,
		}, true)
		transport := client.Transport.(*http.Transport)
		assert.True(transport.TLSClientConfig.InsecureSkipVerify)
		assert.Nil(transport.Proxy)
	})

	t.Run("proxy", func(t *testing.T) {
		client := newClient(&config.AgentConfig{ProxyURL: url}, false)
		transport := client.Transport.(*http.Transport)
		goturl, _ := transport.Proxy(nil)
		assert.False(transport.TLSClientConfig.InsecureSkipVerify)
		assert.Equal("test_url", goturl.String())
	})
}

func TestNewEndpoints(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		e := newEndpoints(&config.AgentConfig{Enabled: false}, "")
		_, ok := e[0].(*nullEndpoint)
		assert.True(t, ok)
	})

	t.Run("panic", func(t *testing.T) {
		for name, tt := range map[string]struct {
			cfg *config.AgentConfig
			err string
		}{
			"key":      {&config.AgentConfig{Enabled: true}, "must have at least one endpoint with key"},
			"key2":     {&config.AgentConfig{Enabled: true, Endpoints: []*config.Endpoint{{Host: "123"}}}, "must have at least one endpoint with key"},
			"endpoint": {&config.AgentConfig{Enabled: true, Endpoints: []*config.Endpoint{{APIKey: "123"}}}, "must have at least one endpoint with key"},
		} {
			t.Run(name, func(t *testing.T) {
				defer func() {
					if e, ok := recover().(error); !ok || e == nil {
						t.Fatal("expected panic")
					} else {
						if e.Error() != tt.err {
							t.Fatalf("invalid error, got %q", e.Error())
						}
					}
				}()
				newEndpoints(tt.cfg, "")
			})
		}
	})

	t.Run("ok", func(t *testing.T) {
		for name, tt := range map[string]struct {
			cfg  *config.AgentConfig
			path string
			exp  []*datadogEndpoint
		}{
			"main": {
				cfg:  &config.AgentConfig{Enabled: true, Endpoints: []*config.Endpoint{{Host: "host1", APIKey: "key1"}}},
				path: "/api/trace",
				exp:  []*datadogEndpoint{{host: "host1", apiKey: "key1", path: "/api/trace"}},
			},
			"additional": {
				cfg: &config.AgentConfig{
					Enabled: true,
					Endpoints: []*config.Endpoint{
						{Host: "host1", APIKey: "key1"},
						{Host: "host2", APIKey: "key2"},
						{Host: "host3", APIKey: "key3"},
						{Host: "host4", APIKey: "key4"},
					},
				},
				path: "/api/trace",
				exp: []*datadogEndpoint{
					{host: "host1", apiKey: "key1", path: "/api/trace"},
					{host: "host2", apiKey: "key2", path: "/api/trace"},
					{host: "host3", apiKey: "key3", path: "/api/trace"},
					{host: "host4", apiKey: "key4", path: "/api/trace"},
				},
			},
		} {
			t.Run(name, func(t *testing.T) {
				assert := assert.New(t)
				e := newEndpoints(tt.cfg, tt.path)
				for i, want := range tt.exp {
					got := e[i].(*datadogEndpoint)
					assert.Equal(want.host, got.host)
					assert.Equal(want.apiKey, got.apiKey)
					assert.Equal(want.path, got.path)
				}
			})
		}
	})

	t.Run("proxy", func(t *testing.T) {
		assert := assert.New(t)
		proxyURL, err := url.Parse("test_url")
		if err != nil {
			t.Fatal(err)
		}
		e := newEndpoints(&config.AgentConfig{
			Enabled:  true,
			ProxyURL: proxyURL,
			Endpoints: []*config.Endpoint{
				{Host: "host1", APIKey: "key1"},
				{Host: "host2", APIKey: "key2"},
				{Host: "host3", APIKey: "key3", NoProxy: true},
			},
		}, "/api/trace")

		// proxy ok
		for _, i := range []int{0, 1} {
			tr := e[i].(*datadogEndpoint).client.Transport.(*http.Transport)
			p, _ := tr.Proxy(nil)
			assert.Equal("test_url", p.String())
		}

		// proxy skipped
		tr := e[2].(*datadogEndpoint).client.Transport.(*http.Transport)
		assert.Nil(tr.Proxy)
	})
}
