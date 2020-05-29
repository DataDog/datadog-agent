package tests

import (
	"bytes"
	"fmt"
	"go/build"
	"os"
	"testing"
	"text/template"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
)

func TestConfig(t *testing.T) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		t.Fatal(err)
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = build.Default.GOPATH
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPolicy": fmt.Sprintf("%s/src/github.com/DataDog/datadog-agent/cmd/agent/dist/conf.d/security_agent.d/runtime-policies.yaml.example", gopath),
	}); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%s", buffer.String())

	aconfig.Datadog.SetConfigType("yaml")
	if err := aconfig.Datadog.ReadConfig(buffer); err != nil {
		t.Fatal(err)
	}

	_, err = module.NewModule(nil)
	if err != nil {
		t.Fatal(err)
	}
}
