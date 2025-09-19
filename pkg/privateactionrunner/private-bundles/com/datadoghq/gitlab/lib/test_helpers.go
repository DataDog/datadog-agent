package lib

import (
	testhelpers "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/test"
	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/types"
	runtimepb "github.com/DataDog/dd-source/domains/actionplatform/proto/runtime"
	"github.com/DataDog/dd-source/domains/workflow/pkg/ids"
)

var testOrgID = ids.OrgID(123)

func NewTestTask(inputs map[string]any) *types.Task {
	return testhelpers.NewTestTask("id", "type", &types.Attributes{
		Name:                  "Test Task",
		BundleID:              "com.test.bundle",
		SecDatadogHeaderValue: "test-header-value",
		Inputs:                inputs,
		OrgId:                 testOrgID,
	})
}

func NewTestCredential(url string) *runtimepb.Credential {
	return &runtimepb.Credential{
		Credential: &runtimepb.Credential_Token{
			Token: &runtimepb.Credential_TokenCredential{
				Tokens: []*runtimepb.Credential_TokenCredential_Token{
					{Name: "gitlabApiToken", Value: "secret"},
					{Name: "baseURL", Value: url},
				},
			},
		},
	}
}
