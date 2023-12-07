package components

import (
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
)

type FakeIntake struct {
	fakeintake.FakeintakeOutput

	client *client.Client
}

var _ e2e.Initializable = &FakeIntake{}

func (fi *FakeIntake) Init(e2e.Context) error {
	fi.client = client.NewClient(fi.URL)
	return nil
}

func (fi *FakeIntake) Client() *client.Client {
	return fi.client
}
