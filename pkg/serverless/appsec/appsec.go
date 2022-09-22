package appsec

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf"
)

type AppSec struct {
	handle *waf.Handle
}

func New() (*AppSec, error) {
	handle, err := waf.NewHandle([]byte(staticRecommendedRules), "", "")
	if err != nil {
		return nil, err
	}
	return &AppSec{
		handle: handle,
	}, nil
}

func (appsec *AppSec) RunWAF(values map[string]interface{}) (matches []byte, err error) {
	ctx := waf.NewContext(appsec.handle)
	defer ctx.Close()
	return ctx.Run(values, 4*time.Millisecond)
}
