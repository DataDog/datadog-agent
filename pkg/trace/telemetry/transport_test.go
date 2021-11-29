package telemetry

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}

func TestBasicReverseProxy(t *testing.T) {
	t.Run("Sets correct headers", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/example", nil)
		if err != nil {
			t.Fatal(err)
		}

		// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
		rr := httptest.NewRecorder()

		rp := NewReverseProxy(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "", req.Header.Get("User-Agent"))
			assert.Regexp(t, regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}), log.Default())

		rp.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
