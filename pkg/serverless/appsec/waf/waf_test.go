// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !windows && (amd64 || arm64) && (linux || darwin)

package waf

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	require.NoError(t, Health())
}

func TestVersion(t *testing.T) {
	require.Regexp(t, `[0-9]+\.[0-9]+\.[0-9]+`, Version())
}

var testArachniRule = newArachniTestRule([]ruleInput{{Address: "server.request.headers.no_cookies", KeyPath: []string{"user-agent"}}}, nil)

var testArachniRuleTmpl = template.Must(template.New("").Parse(`
{
  "version": "2.1",
  "rules": [
	{
	  "id": "ua0-600-12x",
	  "name": "Arachni",
	  "tags": {
		"type": "security_scanner",
		"category": "attack_attempt"
	  },
	  "conditions": [
		{
		  "operator": "match_regex",
		  "parameters": {
			"inputs": [
			{{ range $i, $input := .Inputs -}}
			  {{ if gt $i 0 }},{{ end }}
				{ "address": "{{ $input.Address }}"{{ if ne (len $input.KeyPath) 0 }},  "key_path": [ {{ range $i, $path := $input.KeyPath }}{{ if gt $i 0 }}, {{ end }}"{{ $path }}"{{ end }} ]{{ end }} }
			{{- end }}
			],
			"regex": "^Arachni"
		  }
		}
	  ],
	  "transformers": []
	  {{- if .Actions }},
		"on_match": [
		{{ range $i, $action := .Actions -}}
		  {{ if gt $i 0 }},{{ end }}
		  "{{ $action }}"
		{{- end }}
		]
	  {{- end }}
	}
  ]
}
`))

type ruleInput struct {
	Address string
	KeyPath []string
}

func newArachniTestRule(inputs []ruleInput, actions []string) []byte {
	var buf bytes.Buffer
	if err := testArachniRuleTmpl.Execute(&buf, struct {
		Inputs  []ruleInput
		Actions []string
	}{Inputs: inputs, Actions: actions}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func newDefaultHandle(jsonRule []byte) (*Handle, error) {
	return NewHandle(jsonRule, "", "")
}

func TestNewWAF(t *testing.T) {
	defer requireZeroNBLiveCObjects(t)
	t.Run("valid-rule", func(t *testing.T) {
		waf, err := newDefaultHandle(testArachniRule)
		require.NoError(t, err)
		require.NotNil(t, waf)
		defer waf.Close()
	})

	t.Run("invalid-json", func(t *testing.T) {
		waf, err := newDefaultHandle([]byte(`not json`))
		require.Error(t, err)
		require.Nil(t, waf)
	})

	t.Run("rule-encoding-error", func(t *testing.T) {
		// For now, the null value cannot be encoded into a WAF object representation so it allows us to cover this
		// case where the JSON rule cannot be encoded into a WAF object.
		waf, err := newDefaultHandle([]byte(`null`))
		require.Error(t, err)
		require.Nil(t, waf)
	})

	t.Run("invalid-rule", func(t *testing.T) {
		// Test with a valid JSON but invalid rule format (field events should be an array)
		const rule = `
{
  "version": "2.1",
  "events": [
	{
	  "id": "ua0-600-12x",
	  "name": "Arachni",
	  "tags": {
		"type": "security_scanner"
	  },
	  "conditions": [
		{
		  "operation": "match_regex",
		  "parameters": {
			"inputs": {
			  { "address": "server.request.headers.no_cookies" }
			},
			"regex": "^Arachni"
		  }
		}
	  ],
	  "transformers": []
	}
  ]
}
`
		waf, err := newDefaultHandle([]byte(rule))
		require.Error(t, err)
		require.Nil(t, waf)
	})
}

func TestMatching(t *testing.T) {
	defer requireZeroNBLiveCObjects(t)

	waf, err := newDefaultHandle(newArachniTestRule([]ruleInput{{Address: "my.input"}}, nil))
	require.NoError(t, err)
	require.NotNil(t, waf)

	require.Equal(t, []string{"my.input"}, waf.Addresses())

	wafCtx := NewContext(waf)
	require.NotNil(t, wafCtx)

	// Not matching because the address value doesn't match the rule
	values := map[string]interface{}{
		"my.input": "go client",
	}
	matches, actions, err := wafCtx.Run(values, time.Second)
	require.NoError(t, err)
	require.Nil(t, matches)
	require.Nil(t, actions)

	// Not matching because the address is not used by the rule
	values = map[string]interface{}{
		"server.request.uri.raw": "something",
	}
	matches, actions, err = wafCtx.Run(values, time.Second)
	require.NoError(t, err)
	require.Nil(t, matches)
	require.Nil(t, actions)

	// Not matching due to a timeout
	values = map[string]interface{}{
		"my.input": "Arachni",
	}
	matches, actions, err = wafCtx.Run(values, 0)
	require.Equal(t, ErrTimeout, err)
	require.Nil(t, matches)
	require.Nil(t, actions)

	// Matching
	// Note a WAF rule can only match once. This is why we test the matching case at the end.
	values = map[string]interface{}{
		"my.input": "Arachni",
	}
	matches, actions, err = wafCtx.Run(values, time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	require.Nil(t, actions)

	// Not matching anymore since it already matched before
	matches, actions, err = wafCtx.Run(values, time.Second)
	require.NoError(t, err)
	require.Nil(t, matches)
	require.Nil(t, actions)

	// Nil values
	matches, actions, err = wafCtx.Run(nil, time.Second)
	require.NoError(t, err)
	require.Nil(t, matches)
	require.Nil(t, actions)

	// Empty values
	matches, actions, err = wafCtx.Run(map[string]interface{}{}, time.Second)
	require.NoError(t, err)
	require.Nil(t, matches)

	wafCtx.Close()
	waf.Close()
	// Using the WAF instance after it was closed leads to a nil WAF context
	require.Nil(t, NewContext(waf))
}

func TestActions(t *testing.T) {
	testActions := func(expectedActions []string) func(t *testing.T) {
		return func(t *testing.T) {
			defer requireZeroNBLiveCObjects(t)

			waf, err := newDefaultHandle(newArachniTestRule([]ruleInput{{Address: "my.input"}}, expectedActions))
			require.NoError(t, err)
			require.NotNil(t, waf)
			defer waf.Close()

			wafCtx := NewContext(waf)
			require.NotNil(t, wafCtx)
			defer wafCtx.Close()

			// Not matching because the address value doesn't match the rule
			values := map[string]interface{}{
				"my.input": "Arachni",
			}
			matches, actions, err := wafCtx.Run(values, time.Second)
			require.NoError(t, err)
			require.NotEmpty(t, matches)
			// FIXME: check with libddwaf why the order of returned actions is not kept the same
			require.ElementsMatch(t, expectedActions, actions)
		}
	}

	t.Run("single", testActions([]string{"block"}))
	t.Run("multiple-actions", testActions([]string{"action 1", "action 2", "action 3"}))
}

func TestUpdateRuleData(t *testing.T) {
	defer requireZeroNBLiveCObjects(t)

	var (
		// Sample rules, one blocking IP addresses defined in the rule data `blocked_ips`,
		// and the other blocking users defined in the rule data `blocked_users`.
		testBlockingRule = []byte(`
{
  "version": "2.1",
  "rules": [
	{
		"id": "block_ips",
		"name": "Block IP addresses",
		"tags": {
		  "type": "ip_addresses",
		  "category": "blocking"
		},
		"conditions": [
		  {
			"parameters": {
			  "inputs": [
				{ "address": "http.client_ip" }
			  ],
			  "data": "blocked_ips"
			},
			"operator": "ip_match"
		  }
		],
		"transformers": [],
		"on_match": [
		  "block_ip"
		]
	},

	{
		"id": "block_users",
		"name": "Block authenticated users",
		"tags": {
		  "type": "users",
		  "category": "blocking"
		},
		"conditions": [
		  {
			"parameters": {
			  "inputs": [
				{ "address": "usr.id" }
			  ],
			  "data": "blocked_users"
			},
			"operator": "exact_match"
		  }
		],
		"transformers": [],
		"on_match": [
		  "block_user"
		]
	}
  ],
  "rules_data": [
	{
		"id": "blocked_users",
		"type": "data_with_expiration",
		"data": [
			{ "value": "zouzou" }
		]
	},

	{
		"id": "blocked_ips",
		"type": "ip_with_expiration",
		"data": [
			{ "value": "1.2.3.4" }
		]
	}
  ]
}
`)

		testBlockingRuleData = []byte(`
[
    {
		"id": "blocked_users",
		"type": "data_with_expiration",
		"data": [
			{ "value": "moutix" }
		]
	},

	{
		"id": "blocked_ips",
		"type": "ip_with_expiration",
		"data": [
			{ "value": "10.0.0.1" }
		]
	}
]`)

		testEmptyRuleData = []byte(`
[
    {
		"id": "blocked_users",
		"type": "data_with_expiration",
		"data": []
	},

	{
		"id": "blocked_ips",
		"type": "ip_with_expiration",
		"data": []
	}
]`)
	)

	waf, err := newDefaultHandle(testBlockingRule)
	require.NoError(t, err)
	require.NotNil(t, waf)
	defer waf.Close()

	// Helper function to test that the given address blocks or not.
	// A rule can still only match once per context and so this function helps
	// testing several times the same rule under distinct rule data values.
	test := func(values map[string]interface{}, expectedActions []string) {
		wafCtx := NewContext(waf)
		require.NotNil(t, wafCtx)
		defer wafCtx.Close()

		matches, actions, err := wafCtx.Run(values, time.Second)
		require.NoError(t, err)

		if len(expectedActions) > 0 {
			require.NotEmpty(t, matches)
			require.Equal(t, len(expectedActions), len(actions))
			require.ElementsMatch(t, expectedActions, actions)
		} else {
			require.Nil(t, matches)
			require.Nil(t, actions)
		}
	}

	// Not matching because the address values don't match the rules data
	test(map[string]interface{}{
		"http.client_ip": "10.0.0.1",
		"usr.id":         "moutix",
	}, nil)

	// Matching because the address values match the rules data
	test(map[string]interface{}{
		"http.client_ip": "1.2.3.4",
		"usr.id":         "zouzou",
	}, []string{"block_user", "block_ip"})

	// Update the rules' data
	err = waf.UpdateRuleData(testBlockingRuleData)
	require.NoError(t, err)

	// Not matching because the address values match the updated rules data
	test(map[string]interface{}{
		"http.client_ip": "1.2.3.4",
		"usr.id":         "zouzou",
	}, nil)

	// Matching because the address values don't match the updated rules data
	test(map[string]interface{}{
		"http.client_ip": "10.0.0.1",
		"usr.id":         "moutix",
	}, []string{"block_user", "block_ip"})

	// Empty the rules data so that nothing matches anymore
	err = waf.UpdateRuleData(testEmptyRuleData)
	require.NoError(t, err)

	test(map[string]interface{}{
		"http.client_ip": "1.2.3.4",
		"usr.id":         "zouzou",
	}, nil)

	test(map[string]interface{}{
		"http.client_ip": "10.0.0.1",
		"usr.id":         "moutix",
	}, nil)

	// Update the rules' data again
	err = waf.UpdateRuleData(testBlockingRuleData)
	require.NoError(t, err)

	// Matching because the address values don't match the updated rules data
	test(map[string]interface{}{
		"http.client_ip": "10.0.0.1",
		"usr.id":         "moutix",
	}, []string{"block_user", "block_ip"})
}

func TestAddresses(t *testing.T) {
	defer requireZeroNBLiveCObjects(t)
	expectedAddresses := []string{"my.first.input", "my.second.input", "my.third.input", "my.indexed.input"}
	addresses := []ruleInput{{Address: "my.first.input"}, {Address: "my.second.input"}, {Address: "my.third.input"}, {Address: "my.indexed.input", KeyPath: []string{"indexed"}}}
	waf, err := newDefaultHandle(newArachniTestRule(addresses, nil))
	require.NoError(t, err)
	defer waf.Close()
	require.Equal(t, expectedAddresses, waf.Addresses())
}

func TestConcurrency(t *testing.T) {
	defer requireZeroNBLiveCObjects(t)

	// Start 800 goroutines that will use the WAF 500 times each
	nbUsers := 800
	nbRun := 500

	t.Run("concurrent-waf-release", func(t *testing.T) {
		waf, err := newDefaultHandle(testArachniRule)
		require.NoError(t, err)

		wafCtx := NewContext(waf)
		require.NotNil(t, wafCtx)

		var (
			closed uint32
			done   sync.WaitGroup
		)
		done.Add(1)
		go func() {
			defer done.Done()
			// The implementation currently blocks until the WAF contexts get released
			waf.Close()
			atomic.AddUint32(&closed, 1)
		}()

		// The WAF context is not released so waf.Close() should block and `closed` still be 0
		assert.Equal(t, uint32(0), atomic.LoadUint32(&closed))
		// Release the WAF context, which should unlock the previous waf.Close() call
		wafCtx.Close()
		// Now that the WAF context is closed, wait for the goroutine to close the WAF handle.
		done.Wait()
		require.Equal(t, uint32(1), atomic.LoadUint32(&closed))
	})

	t.Run("concurrent-waf-context-usage", func(t *testing.T) {
		waf, err := newDefaultHandle(testArachniRule)
		require.NoError(t, err)
		defer waf.Close()

		wafCtx := NewContext(waf)
		defer wafCtx.Close()

		// User agents that won't match the rule so that it doesn't get pruned.
		// Said otherwise, the User-Agent rule will run as long as it doesn't match, otherwise it gets ignored.
		// This is the reason why the following user agent are not Arachni.
		userAgents := [...]string{"Foo", "Bar", "Datadog"}
		length := len(userAgents)

		var startBarrier, stopBarrier sync.WaitGroup
		// Create a start barrier to synchronize every goroutine's launch and
		// increase the chances of parallel accesses
		startBarrier.Add(1)
		// Create a stopBarrier to signal when all user goroutines are done.
		stopBarrier.Add(nbUsers + 1 /* the extra rules-data-update goroutine*/)

		for n := 0; n < nbUsers; n++ {
			go func() {
				startBarrier.Wait()      // Sync the starts of the goroutines
				defer stopBarrier.Done() // Signal we are done when returning

				for c := 0; c < nbRun; c++ {
					i := c % length
					data := map[string]interface{}{
						"server.request.headers.no_cookies": map[string]string{
							"user-agent": userAgents[i],
						},
					}
					matches, _, err := wafCtx.Run(data, time.Minute)
					if err != nil {
						panic(err)
						return
					}
					if len(matches) > 0 {
						panic(fmt.Errorf("c=%d matches=`%v`", c, string(matches)))
					}
				}
			}()
		}

		// Concurrently update the rules' data from times to times
		go func() {
			startBarrier.Wait()      // Sync the starts of the goroutines
			defer stopBarrier.Done() // Signal we are done when returning

			for c := 0; c < nbRun; c++ {
				if err := waf.UpdateRuleData([]byte(`[]`)); err != nil {
					panic(err)
				}
				time.Sleep(time.Microsecond) // This is going to be more than this when under pressure
			}
		}()

		// Save the test start time to compare it to the first metrics store's
		// that should be latter.
		startBarrier.Done() // Unblock the user goroutines
		stopBarrier.Wait()  // Wait for the user goroutines to be done

		// Test the rule matches Arachni in the end
		data := map[string]interface{}{
			"server.request.headers.no_cookies": map[string]string{
				"user-agent": "Arachni",
			},
		}
		matches, _, err := wafCtx.Run(data, time.Second)
		require.NoError(t, err)
		require.NotEmpty(t, matches)
	})

	t.Run("concurrent-waf-instance-usage", func(t *testing.T) {
		waf, err := newDefaultHandle(testArachniRule)
		require.NoError(t, err)
		defer waf.Close()

		// User agents that won't match the rule so that it doesn't get pruned.
		// Said otherwise, the User-Agent rule will run as long as it doesn't match, otherwise it gets ignored.
		// This is the reason why the following user agent are not Arachni.
		userAgents := [...]string{"Foo", "Bar", "Datadog"}
		length := len(userAgents)

		var startBarrier, stopBarrier sync.WaitGroup
		// Create a start barrier to synchronize every goroutine's launch and
		// increase the chances of parallel accesses
		startBarrier.Add(1)
		// Create a stopBarrier to signal when all user goroutines are done.
		stopBarrier.Add(nbUsers)

		for n := 0; n < nbUsers; n++ {
			go func() {
				startBarrier.Wait()      // Sync the starts of the goroutines
				defer stopBarrier.Done() // Signal we are done when returning

				wafCtx := NewContext(waf)
				defer wafCtx.Close()

				for c := 0; c < nbRun; c++ {
					i := c % length
					data := map[string]interface{}{
						"server.request.headers.no_cookies": map[string]string{
							"user-agent": userAgents[i],
						},
					}
					matches, _, err := wafCtx.Run(data, time.Minute)
					if err != nil {
						panic(err)
					}
					if len(matches) > 0 {
						panic(fmt.Errorf("c=%d matches=`%v`", c, string(matches)))
					}
				}

				// Test the rule matches Arachni in the end
				data := map[string]interface{}{
					"server.request.headers.no_cookies": map[string]string{
						"user-agent": "Arachni",
					},
				}
				matches, actions, err := wafCtx.Run(data, time.Second)
				require.NoError(t, err)
				require.NotEmpty(t, matches)
				require.Nil(t, actions)
			}()
		}

		// Save the test start time to compare it to the first metrics store's
		// that should be latter.
		startBarrier.Done() // Unblock the user goroutines
		stopBarrier.Wait()  // Wait for the user goroutines to be done
	})
}

func TestRunError(t *testing.T) {
	for _, tc := range []struct {
		Err            error
		ExpectedString string
	}{
		{
			Err:            ErrInternal,
			ExpectedString: "internal waf error",
		},
		{
			Err:            ErrTimeout,
			ExpectedString: "waf timeout",
		},
		{
			Err:            ErrInvalidObject,
			ExpectedString: "invalid waf object",
		},
		{
			Err:            ErrInvalidArgument,
			ExpectedString: "invalid waf argument",
		},
		{
			Err:            ErrOutOfMemory,
			ExpectedString: "out of memory",
		},
		{
			Err:            RunError(33),
			ExpectedString: "unknown waf error 33",
		},
	} {
		t.Run(tc.ExpectedString, func(t *testing.T) {
			require.Equal(t, tc.ExpectedString, tc.Err.Error())
		})
	}
}

func TestMetrics(t *testing.T) {
	rules := `
{
  "version": "2.1",
  "metadata": {
	"rules_version": "1.2.7"
  },
  "rules": [
	{
	  "id": "valid-rule",
	  "name": "Unicode Full/Half Width Abuse Attack Attempt",
	  "tags": {
		"type": "http_protocol_violation"
	  },
	  "conditions": [
		{
		  "parameters": {
			"inputs": [
			  {
				"address": "server.request.uri.raw"
			  }
			],
			"regex": "\\%u[fF]{2}[0-9a-fA-F]{2}"
		  },
		  "operator": "match_regex"
		}
	  ],
	  "transformers": []
	},
	{
	  "id": "missing-tags-1",
	  "name": "Unicode Full/Half Width Abuse Attack Attempt",
	  "conditions": [
	  ],
	  "transformers": []
	},
	{
	  "id": "missing-tags-2",
	  "name": "Unicode Full/Half Width Abuse Attack Attempt",
	  "conditions": [
	  ],
	  "transformers": []
	},
	{
	  "id": "missing-name",
	  "tags": {
		"type": "http_protocol_violation"
	  },
	  "conditions": [
	  ],
	  "transformers": []
	}
  ]
}
`
	waf, err := newDefaultHandle([]byte(rules))
	require.NoError(t, err)
	defer waf.Close()
	// TODO: (Francois Mazeau) see if we can make this test more configurable to future proof against libddwaf changes
	t.Run("RulesetInfo", func(t *testing.T) {
		rInfo := waf.RulesetInfo()
		require.Equal(t, uint16(3), rInfo.Failed)
		require.Equal(t, uint16(1), rInfo.Loaded)
		require.Equal(t, 2, len(rInfo.Errors))
		require.Equal(t, "1.2.7", rInfo.Version)
		require.Equal(t, map[string]interface{}{
			"missing key 'tags'": []interface{}{"missing-tags-1", "missing-tags-2"},
			"missing key 'name'": []interface{}{"missing-name"},
		}, rInfo.Errors)
	})

	t.Run("RunDuration", func(t *testing.T) {
		wafCtx := NewContext(waf)
		require.NotNil(t, wafCtx)
		defer wafCtx.Close()
		// Craft matching data to force work on the WAF
		data := map[string]interface{}{
			"server.request.uri.raw": "\\%uff00",
		}
		start := time.Now()
		matches, actions, err := wafCtx.Run(data, time.Second)
		elapsedNS := time.Since(start).Nanoseconds()
		require.NoError(t, err)
		require.NotNil(t, matches)
		require.Nil(t, actions)

		// Make sure that WAF runtime was set
		overall, internal := wafCtx.TotalRuntime()
		require.Greater(t, overall, uint64(0))
		require.Greater(t, internal, uint64(0))
		require.Greater(t, overall, internal)
		require.LessOrEqual(t, overall, uint64(elapsedNS))
	})

	t.Run("Timeouts", func(t *testing.T) {
		wafCtx := NewContext(waf)
		require.NotNil(t, wafCtx)
		defer wafCtx.Close()
		// Craft matching data to force work on the WAF
		data := map[string]interface{}{
			"server.request.uri.raw": "\\%uff00",
		}

		for i := uint64(1); i <= 10; i++ {
			_, _, err := wafCtx.Run(data, time.Nanosecond)
			require.Equal(t, err, ErrTimeout)
			require.Equal(t, i, wafCtx.TotalTimeouts())
		}
	})
}

func requireZeroNBLiveCObjects(t testing.TB) {
	require.Equal(t, uint64(0), atomic.LoadUint64(&nbLiveCObjects))
}

func TestEncoder(t *testing.T) {
	for _, tc := range []struct {
		Name                   string
		Data                   interface{}
		ExpectedError          error
		ExpectedWAFValueType   int
		ExpectedWAFValueLength int
		ExpectedWAFString      string
		MaxValueDepth          interface{}
		MaxArrayLength         interface{}
		MaxMapLength           interface{}
		MaxStringLength        interface{}
	}{
		{
			Name:          "unsupported type",
			Data:          make(chan struct{}),
			ExpectedError: errUnsupportedValue,
		},
		{
			Name:              "string",
			Data:              "hello, waf",
			ExpectedWAFString: "hello, waf",
		},
		{
			Name:                   "string",
			Data:                   "",
			ExpectedWAFValueType:   wafStringType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:              "byte-slice",
			Data:              []byte("hello, waf"),
			ExpectedWAFString: "hello, waf",
		},
		{
			Name:                   "nil-byte-slice",
			Data:                   []byte(nil),
			ExpectedWAFValueType:   wafStringType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:                   "map-with-empty-key-string",
			Data:                   map[string]int{"": 1},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:                   "empty-struct",
			Data:                   struct{}{},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name: "empty-struct-with-private-fields",
			Data: struct {
				a string
				b int
				c bool
			}{},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:          "nil-interface-value",
			Data:          nil,
			ExpectedError: errUnsupportedValue,
		},
		{
			Name:          "nil-pointer-value",
			Data:          (*string)(nil),
			ExpectedError: errUnsupportedValue,
		},
		{
			Name:          "nil-pointer-value",
			Data:          (*int)(nil),
			ExpectedError: errUnsupportedValue,
		},
		{
			Name:              "non-nil-pointer-value",
			Data:              new(int),
			ExpectedWAFString: "0",
		},
		{
			Name:                   "non-nil-pointer-value",
			Data:                   new(string),
			ExpectedWAFValueType:   wafStringType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:                   "having-an-empty-map",
			Data:                   map[string]interface{}{},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:          "unsupported",
			Data:          func() {},
			ExpectedError: errUnsupportedValue,
		},
		{
			Name:              "int",
			Data:              int(1234),
			ExpectedWAFString: "1234",
		},
		{
			Name:              "uint",
			Data:              uint(9876),
			ExpectedWAFString: "9876",
		},
		{
			Name:              "bool",
			Data:              true,
			ExpectedWAFString: "true",
		},
		{
			Name:              "bool",
			Data:              false,
			ExpectedWAFString: "false",
		},
		{
			Name:              "float",
			Data:              33.12345,
			ExpectedWAFString: "33",
		},
		{
			Name:              "float",
			Data:              33.62345,
			ExpectedWAFString: "34",
		},
		{
			Name:                   "slice",
			Data:                   []interface{}{33.12345, "ok", 27},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 3,
		},
		{
			Name:                   "slice-having-unsupported-values",
			Data:                   []interface{}{33.12345, func() {}, "ok", 27, nil},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 3,
		},
		{
			Name:                   "array",
			Data:                   [...]interface{}{func() {}, 33.12345, "ok", 27},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 3,
		},
		{
			Name:                   "map",
			Data:                   map[string]interface{}{"k1": 1, "k2": "2"},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 2,
		},
		{
			Name:                   "map-with-unsupported-key-values",
			Data:                   map[interface{}]interface{}{"k1": 1, 27: "int key", "k2": "2"},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 2,
		},
		{
			Name:                   "map-with-indirect-key-string-values",
			Data:                   map[interface{}]interface{}{"k1": 1, new(string): "string pointer key", "k2": "2"},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 3,
		},
		{
			Name: "struct",
			Data: struct {
				Public  string
				private string
				a       string
				A       string
			}{
				Public:  "Public",
				private: "private",
				a:       "a",
				A:       "A",
			},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 2, // public fields only
		},
		{
			Name: "struct-with-unsupported-values",
			Data: struct {
				Public  string
				private string
				a       string
				A       func()
			}{
				Public:  "Public",
				private: "private",
				a:       "a",
				A:       nil,
			},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 1, // public fields of supported types
		},
		{
			Name:                   "array-max-depth",
			MaxValueDepth:          0,
			Data:                   []interface{}{1, 2, 3, 4, []int{1, 2, 3, 4}},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 4,
		},
		{
			Name:                   "array-max-depth",
			MaxValueDepth:          1,
			Data:                   []interface{}{1, 2, 3, 4, []int{1, 2, 3, 4}},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 5,
		},
		{
			Name:                   "array-max-depth",
			MaxValueDepth:          0,
			Data:                   []interface{}{},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:                   "map-max-depth",
			MaxValueDepth:          0,
			Data:                   map[string]interface{}{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": map[string]string{}},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 4,
		},
		{
			Name:                   "map-max-depth",
			MaxValueDepth:          1,
			Data:                   map[string]interface{}{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": map[string]string{}},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 5,
		},
		{
			Name:                   "map-max-depth",
			MaxValueDepth:          0,
			Data:                   map[string]interface{}{},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:                   "struct-max-depth",
			MaxValueDepth:          0,
			Data:                   struct{}{},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name:          "struct-max-depth",
			MaxValueDepth: 0,
			Data: struct {
				F0 string
				F1 struct{}
			}{},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:          "struct-max-depth",
			MaxValueDepth: 1,
			Data: struct {
				F0 string
				F1 struct{}
			}{},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 2,
		},
		{
			Name:              "scalar-values-max-depth-not-accounted",
			MaxValueDepth:     0,
			Data:              -1234,
			ExpectedWAFString: "-1234",
		},
		{
			Name:              "scalar-values-max-depth-not-accounted",
			MaxValueDepth:     0,
			Data:              uint(1234),
			ExpectedWAFString: "1234",
		},
		{
			Name:              "scalar-values-max-depth-not-accounted",
			MaxValueDepth:     0,
			Data:              false,
			ExpectedWAFString: "false",
		},
		{
			Name:                   "array-max-length",
			MaxArrayLength:         3,
			Data:                   []interface{}{1, 2, 3, 4, 5},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 3,
		},
		{
			Name:                   "map-max-length",
			MaxMapLength:           3,
			Data:                   map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5"},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 3,
		},
		{
			Name:              "string-max-length",
			MaxStringLength:   3,
			Data:              "123456789",
			ExpectedWAFString: "123",
		},
		{
			Name:                   "string-max-length-truncation-leading-to-same-map-keys",
			MaxStringLength:        1,
			Data:                   map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5"},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 5,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         1,
			Data:                   []interface{}{"supported", func() {}, "supported", make(chan struct{})},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         2,
			Data:                   []interface{}{"supported", func() {}, "supported", make(chan struct{})},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 2,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         1,
			Data:                   []interface{}{func() {}, "supported", make(chan struct{}), "supported"},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         2,
			Data:                   []interface{}{func() {}, "supported", make(chan struct{}), "supported"},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 2,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         1,
			Data:                   []interface{}{func() {}, make(chan struct{}), "supported"},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         3,
			Data:                   []interface{}{"supported", func() {}, make(chan struct{})},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         3,
			Data:                   []interface{}{func() {}, "supported", make(chan struct{})},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         3,
			Data:                   []interface{}{func() {}, make(chan struct{}), "supported"},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name:                   "unsupported-array-values",
			MaxArrayLength:         2,
			Data:                   []interface{}{func() {}, make(chan struct{}), "supported", "supported"},
			ExpectedWAFValueType:   wafArrayType,
			ExpectedWAFValueLength: 2,
		},
		{
			Name: "unsupported-map-key-types",
			Data: map[interface{}]int{
				"supported":           1,
				interface{ m() }(nil): 1,
				nil:                   1,
				(*int)(nil):           1,
				(*string)(nil):        1,
				make(chan struct{}):   1,
			},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 1,
		},
		{
			Name: "unsupported-map-key-types",
			Data: map[interface{}]int{
				interface{ m() }(nil): 1,
				nil:                   1,
				(*int)(nil):           1,
				(*string)(nil):        1,
				make(chan struct{}):   1,
			},
			ExpectedWAFValueType:   wafMapType,
			ExpectedWAFValueLength: 0,
		},
		{
			Name: "unsupported-map-values",
			Data: map[string]interface{}{
				"k0": "supported",
				"k1": func() {},
				"k2": make(chan struct{}),
			},
			MaxMapLength:           3,
			ExpectedWAFValueLength: 1,
		},
		{
			Name: "unsupported-map-values",
			Data: map[string]interface{}{
				"k0": "supported",
				"k1": "supported",
				"k2": make(chan struct{}),
			},
			MaxMapLength:           3,
			ExpectedWAFValueLength: 2,
		},
		{
			Name: "unsupported-map-values",
			Data: map[string]interface{}{
				"k0": "supported",
				"k1": "supported",
				"k2": make(chan struct{}),
			},
			MaxMapLength:           1,
			ExpectedWAFValueLength: 1,
		},
		{
			Name: "unsupported-struct-values",
			Data: struct {
				F0 string
				F1 func()
				F2 chan struct{}
			}{
				F0: "supported",
				F1: func() {},
				F2: make(chan struct{}),
			},
			MaxMapLength:           3,
			ExpectedWAFValueLength: 1,
		},
		{
			Name: "unsupported-map-values",
			Data: struct {
				F0 string
				F1 string
				F2 chan struct{}
			}{
				F0: "supported",
				F1: "supported",
				F2: make(chan struct{}),
			},
			MaxMapLength:           3,
			ExpectedWAFValueLength: 2,
		},
		{
			Name: "unsupported-map-values",
			Data: struct {
				F0 string
				F1 string
				F2 chan struct{}
			}{
				F0: "supported",
				F1: "supported",
				F2: make(chan struct{}),
			},
			MaxMapLength:           1,
			ExpectedWAFValueLength: 1,
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			defer requireZeroNBLiveCObjects(t)

			maxValueDepth := 10
			if max := tc.MaxValueDepth; max != nil {
				maxValueDepth = max.(int)
			}
			maxArrayLength := 1000
			if max := tc.MaxArrayLength; max != nil {
				maxArrayLength = max.(int)
			}
			maxMapLength := 1000
			if max := tc.MaxMapLength; max != nil {
				maxMapLength = max.(int)
			}
			maxStringLength := 4096
			if max := tc.MaxStringLength; max != nil {
				maxStringLength = max.(int)
			}
			e := encoder{
				maxDepth:        maxValueDepth,
				maxStringLength: maxStringLength,
				maxArrayLength:  maxArrayLength,
				maxMapLength:    maxMapLength,
			}
			wo, err := e.encode(tc.Data)
			if tc.ExpectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.ExpectedError, err)
				require.Nil(t, wo)
				return
			}
			defer freeWO(wo)

			require.NoError(t, err)
			require.NotEqual(t, &wafObject{}, wo)

			if tc.ExpectedWAFValueType != 0 {
				require.Equal(t, tc.ExpectedWAFValueType, int(wo._type), "bad waf value type")
			}
			if tc.ExpectedWAFValueLength != 0 {
				require.Equal(t, tc.ExpectedWAFValueLength, int(wo.nbEntries), "bad waf value length")
			}
			if expectedStr := tc.ExpectedWAFString; expectedStr != "" {
				require.Equal(t, wafStringType, int(wo._type), "bad waf string value type")
				cbuf := uintptr(unsafe.Pointer(*wo.stringValuePtr()))
				gobuf := []byte(expectedStr)
				require.Equal(t, len(gobuf), int(wo.nbEntries), "bad waf value length")
				for i, gobyte := range gobuf {
					// Go pointer arithmetic for cbyte := cbuf[i]
					cbyte := *(*uint8)(unsafe.Pointer(cbuf + uintptr(i)))
					if cbyte != gobyte {
						t.Fatalf("bad waf string value content: i=%d cbyte=%d gobyte=%d", i, cbyte, gobyte)
					}
				}
			}

			// Pass the encoded value to the WAF to make sure it doesn't return an error
			waf, err := newDefaultHandle(newArachniTestRule([]ruleInput{{Address: "my.input"}}, nil))
			require.NoError(t, err)
			defer waf.Close()
			wafCtx := NewContext(waf)
			require.NotNil(t, wafCtx)
			defer wafCtx.Close()
			_, _, err = wafCtx.Run(map[string]interface{}{
				"my.input": tc.Data,
			}, time.Second)
			require.NoError(t, err)
		})
	}
}

// This test needs a working encoder to function properly, as it first encodes the objects before decoding them
func TestDecoder(t *testing.T) {
	const intSize = 32 << (^uint(0) >> 63) // copied from recent versions of math.MaxInt
	const maxInt = 1<<(intSize-1) - 1      // copied from recent versions of math.MaxInt
	e := encoder{
		maxDepth:        maxInt,
		maxStringLength: maxInt,
		maxArrayLength:  maxInt,
		maxMapLength:    maxInt,
	}
	objBuilder := func(v interface{}) *wafObject {
		var err error
		obj := &wafObject{}
		// Right now the encoder encodes integer values as strings to match the WAF representation.
		// We circumvent this here by manually encoding so that we can test with WAF objects that hold real integers,
		// not string representations of integers. See https://github.com/DataDog/libddwaf/issues/41.
		if v, ok := v.(int64); ok {
			obj.setInt64(toCInt64(int(v)))
			return obj
		}
		if v, ok := v.(uint64); ok {
			obj.setUint64(toCUint64(uint(v)))
			return obj
		}
		obj, err = e.encode(v)
		require.NoError(t, err, "Encoding object failed")
		return obj
	}

	t.Run("Valid", func(t *testing.T) {
		for _, tc := range []struct {
			Name          string
			Object        *wafObject
			ExpectedValue interface{}
		}{
			{
				Name:          "string",
				ExpectedValue: "string",
				Object:        objBuilder("string"),
			},
			{
				Name:          "empty-string",
				ExpectedValue: "",
				Object:        objBuilder(""),
			},
			{
				Name:          "uint64",
				ExpectedValue: uint64(42),
				Object:        objBuilder(uint64(42)),
			},
			{
				Name:          "int64",
				ExpectedValue: int64(42),
				Object:        objBuilder(int64(42)),
			},
			{
				Name:          "array",
				ExpectedValue: []interface{}{"str1", "str2", "str3", "str4"},
				Object:        objBuilder([]string{"str1", "str2", "str3", "str4"}),
			},
			{
				Name:          "empty-array",
				ExpectedValue: []interface{}{},
				Object:        objBuilder([]interface{}{}),
			},
			{
				Name:          "struct",
				ExpectedValue: map[string]interface{}{"Str": "string"},
				Object: objBuilder(struct {
					Str string
				}{Str: "string"}),
			},
			{
				Name:          "empty-struct",
				ExpectedValue: map[string]interface{}{},
				Object:        objBuilder(struct{}{}),
			},
			{
				Name:          "map",
				ExpectedValue: map[string]interface{}{"foo": "bar", "bar": "baz", "baz": "foo"},
				Object:        objBuilder(map[string]interface{}{"foo": "bar", "bar": "baz", "baz": "foo"}),
			},
			{
				Name:          "empty-map",
				ExpectedValue: map[string]interface{}{},
				Object:        objBuilder(map[string]interface{}{}),
			},
			{
				Name:          "nested",
				ExpectedValue: []interface{}{"1", "2", map[string]interface{}{"foo": "bar", "bar": "baz", "baz": "foo"}, []interface{}{"1", "2", "3"}},
				Object:        objBuilder([]interface{}{1, "2", map[string]string{"foo": "bar", "bar": "baz", "baz": "foo"}, []int{1, 2, 3}}),
			},
		} {
			tc := tc
			t.Run(tc.Name, func(t *testing.T) {
				defer freeWO(tc.Object)
				val, err := decodeObject(tc.Object)
				require.NoErrorf(t, err, "Error decoding the object: %v", err)
				require.Equal(t, reflect.TypeOf(tc.ExpectedValue), reflect.TypeOf(val))
				require.Equal(t, tc.ExpectedValue, val)
			})
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		for _, tc := range []struct {
			Name          string
			Object        *wafObject
			Modifier      func(object *wafObject)
			ExpectedError error
		}{
			{
				Name:          "WAF-object",
				Object:        nil,
				ExpectedError: errNilObjectPtr,
			},
			{
				Name:          "type",
				Object:        objBuilder("obj"),
				Modifier:      func(object *wafObject) { object._type = 5 },
				ExpectedError: errUnsupportedValue,
			},
			{
				Name:          "map-key-1",
				Object:        objBuilder(map[string]interface{}{"baz": "foo"}),
				Modifier:      func(object *wafObject) { object.index(0).setMapKey(nil, 0) },
				ExpectedError: errInvalidMapKey,
			},
			{
				Name:          "map-key-2",
				Object:        objBuilder(map[string]interface{}{"baz": "foo"}),
				Modifier:      func(object *wafObject) { object.index(0).setMapKey(nil, 10) },
				ExpectedError: errInvalidMapKey,
			},
			{
				Name:          "array-ptr",
				Object:        objBuilder([]interface{}{"foo"}),
				Modifier:      func(object *wafObject) { *object.arrayValuePtr() = nil },
				ExpectedError: errNilObjectPtr,
			},
			{
				Name:          "map-ptr",
				Object:        objBuilder(map[string]interface{}{"baz": "foo"}),
				Modifier:      func(object *wafObject) { *object.arrayValuePtr() = nil },
				ExpectedError: errNilObjectPtr,
			},
		} {
			tc := tc
			t.Run(tc.Name, func(t *testing.T) {
				if tc.Modifier != nil {
					tc.Modifier(tc.Object)
				}
				_, err := decodeObject(tc.Object)
				if tc.ExpectedError != nil {
					require.Equal(t, tc.ExpectedError, err)
				} else {
					require.Error(t, err)
				}

			})
		}
	})
}

func TestObfuscatorConfig(t *testing.T) {
	rule := newArachniTestRule([]ruleInput{{Address: "my.addr", KeyPath: []string{"key"}}}, nil)
	t.Run("key", func(t *testing.T) {
		waf, err := NewHandle(rule, "key", "")
		require.NoError(t, err)
		defer waf.Close()
		wafCtx := NewContext(waf)
		require.NotNil(t, wafCtx)
		defer wafCtx.Close()
		data := map[string]interface{}{
			"my.addr": map[string]interface{}{"key": "Arachni-sensitive-Arachni"},
		}
		matches, actions, err := wafCtx.Run(data, time.Second)
		require.NotNil(t, matches)
		require.Nil(t, actions)
		require.NoError(t, err)
		require.NotContains(t, (string)(matches), "sensitive")
	})

	t.Run("val", func(t *testing.T) {
		waf, err := NewHandle(rule, "", "sensitive")
		require.NoError(t, err)
		defer waf.Close()
		wafCtx := NewContext(waf)
		require.NotNil(t, wafCtx)
		defer wafCtx.Close()
		data := map[string]interface{}{
			"my.addr": map[string]interface{}{"key": "Arachni-sensitive-Arachni"},
		}
		matches, actions, err := wafCtx.Run(data, time.Second)
		require.NotNil(t, matches)
		require.Nil(t, actions)
		require.NoError(t, err)
		require.NotContains(t, (string)(matches), "sensitive")
	})

	t.Run("off", func(t *testing.T) {
		waf, err := NewHandle(rule, "", "")
		require.NoError(t, err)
		defer waf.Close()
		wafCtx := NewContext(waf)
		require.NotNil(t, wafCtx)
		defer wafCtx.Close()
		data := map[string]interface{}{
			"my.addr": map[string]interface{}{"key": "Arachni-sensitive-Arachni"},
		}
		matches, actions, err := wafCtx.Run(data, time.Second)
		require.NotNil(t, matches)
		require.Nil(t, actions)
		require.NoError(t, err)
		require.Contains(t, (string)(matches), "sensitive")
	})
}

func TestFree(t *testing.T) {
	t.Run("nil-value", func(t *testing.T) {
		require.NotPanics(t, func() {
			freeWO(nil)
		})
	})

	t.Run("zero-value", func(t *testing.T) {
		require.NotPanics(t, func() {
			freeWO(&wafObject{})
		})
	})
}

func BenchmarkEncoder(b *testing.B) {
	defer requireZeroNBLiveCObjects(b)

	rnd := rand.New(rand.NewSource(33))
	buf := make([]byte, 16384)
	n, err := rnd.Read(buf)
	fullstr := string(buf)
	encoder := encoder{
		maxDepth:        10,
		maxStringLength: 1 * 1024 * 1024,
		maxArrayLength:  100,
		maxMapLength:    100,
	}
	for _, l := range []int{1024, 4096, 8192, 16384} {
		b.Run(fmt.Sprintf("%d", l), func(b *testing.B) {
			str := fullstr[:l]
			slice := []string{str, str, str, str, str, str, str, str, str, str}
			data := map[string]interface{}{
				"k0": slice,
				"k1": slice,
				"k2": slice,
				"k3": slice,
				"k4": slice,
				"k5": slice,
				"k6": slice,
				"k7": slice,
				"k8": slice,
				"k9": slice,
			}
			if err != nil || n != len(buf) {
				b.Fatal(err)
			}
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				v, err := encoder.encode(data)
				if err != nil {
					b.Fatal(err)
				}
				freeWO(v)
			}
		})
	}
}
