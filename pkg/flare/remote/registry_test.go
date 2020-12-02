package remote

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRegistration(t *testing.T) {
	id := "id"
	source := "foo"
	service := "bar"
	env := "test"

	s := RegisterSource(id, source, service, env)

	assert.Equal(t, s.Id, id)
	assert.Equal(t, s.Source, source)
	assert.Equal(t, s.Service, service)
	assert.Equal(t, s.Env, env)

	s, ok := GetSourceById(id)
	assert.NotNil(t, s)
	assert.Equal(t, true, ok)
	assert.Equal(t, id, s.Id)
	assert.Equal(t, source, s.Source)
	assert.Equal(t, service, s.Service)
	assert.Equal(t, env, s.Env)

	// expire source (ie. no heartbeat situation)
	registrationMap.Set(id, s, 100*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	s, ok = GetSourceById(id)
	assert.Nil(t, s)
	assert.Equal(t, ok, false)

	// non-existant source
	s2, ok := GetSourceById("nothere")
	assert.Nil(t, s2)
	assert.Equal(t, ok, false)

}

func TestFilters(t *testing.T) {
	id := "id"
	source := "foo"
	service := "bar"
	env := "test"

	for i := 0; i < 10; i++ {
		for j := 'a'; j <= 'b'; j++ {
			RegisterSource(
				fmt.Sprintf("%s_%c_%d", id, j, i),
				source,
				service,
				fmt.Sprintf("%s_%c", env, j))
		}
	}

	sources := GetSourcesByServiceAndEnv("bar", "")
	assert.Equal(t, 20, len(sources))
	sources = GetSourcesByServiceAndEnv("bar", "test_a")
	assert.Equal(t, 10, len(sources))
	for _, s := range sources {
		assert.Equal(t, s.Source, "foo")
		assert.Equal(t, s.Service, "bar")
		assert.Equal(t, s.Env, "test_a")
	}

	// ...
}
