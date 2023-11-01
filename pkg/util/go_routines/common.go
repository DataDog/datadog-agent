package go_routines

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// GetGoRoutinesDump returns the stack trace of every Go routine of a running Agent.
func GetGoRoutinesDump(cfg pkgconfigmodel.Reader) (string, error) {
	ipcAddress, err := pkgconfigmodel.GetIPCAddress(cfg)
	if err != nil {
		return "", err
	}

	pprofURL := fmt.Sprintf("http://%v:%s/debug/pprof/goroutine?debug=2",
		ipcAddress, cfg.GetString("expvar_port"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := http.Client{}
	req, err := http.NewRequest(http.MethodGet, pprofURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return string(data), err
}
