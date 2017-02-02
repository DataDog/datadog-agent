package ec2

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var ecdInstanceString = "i-0123456789abcdef0"

func TestIsDefaultHostname(t *testing.T) {
	assert.True(t, IsDefaultHostname("IP-FOO"))
	assert.True(t, IsDefaultHostname("domuarigato"))
	assert.False(t, IsDefaultHostname(""))
}

func TestGetInstanceID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, ecdInstanceString)
	}))
	defer ts.Close()

	val, err := getInstanceID(ts.URL)
	assert.Nil(t, err)
	assert.Equal(t, ecdInstanceString, val)
}
