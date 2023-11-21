package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/rc-updater/catalog"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
)

const agentInstallPath = "/tmp/agent/"

// TODO
// * updater should only fetch version/catalog and notify back
// * a version manager does the version handling
//   version cleanup

func main() {
	var err error

	ctx, cc := context.WithCancel(context.Background())
	defer cc()

	common.SetupConfigWithWarnings("/etc/datadog-agent/datadog.yaml", "")

	// TODO fs path traversal! yay
	// it has to be outside of the datadog installation folder or with the
	// current hacky solution it will create /opt/datadog-agent and we can't
	// link it
	service, err := service.NewService("../../update-client.db")
	if err != nil {
		log.Fatal(err)
	}
	service.Start(ctx)

	updateChan := make(chan Config, 1)
	defer close(updateChan)

	updater, err := NewUpdater(ctx, service, agentInstallPath, updateChan)
	if err != nil {
		log.Fatalf("err: %s", err.Error())
	}
	err = updater.Start()
	if err != nil {
		log.Fatalf("err: %s", err.Error())
	}
	defer updater.Close()

	versionManager := NewVersionManager(updater, catalog.GetMockedCatalog(), updateChan)
	err = versionManager.Start()
	if err != nil {
		log.Fatalf("error: %s", err.Error())
	}
	defer versionManager.Stop()

	cancelChan := make(chan os.Signal, 1)
	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)
	sig := <-cancelChan
	log.Printf("Caught signal %v", sig)
}
