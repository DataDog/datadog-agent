// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package shellscript

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const loaderName string = "shellscript"

type shellScriptLoader struct {
	tagger tagger.Component
}

func newLoader(_ sender.SenderManager, _ option.Option[integrations.Component], tagger tagger.Component) (check.Loader, error) {
	return &shellScriptLoader{
		tagger: tagger,
	}, nil
}

// Name returns Shared Library loader name
func (*shellScriptLoader) Name() string {
	return loaderName
}

func (sl *shellScriptLoader) String() string {
	return "Sheelscript Loader"
}

// Load returns a Shared Library check
func (sl *shellScriptLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	shellscript := filepath.Join(defaultpaths.GetDistPath(), "scripts", fmt.Sprintf("%s.sh", config.Name))

	info, err := os.Stat(shellscript)

	if err != nil {
		return nil, fmt.Errorf("shellscript '%s' not available at: %s", config.Name, shellscript)
	}

	filePerimission := info.Mode().Perm()
	if !(filePerimission&0111 == 0111) {
		return nil, fmt.Errorf("shellscript '%s' is not executable", config.Name)
	}

	c, err := newCheck(senderManager, sl.tagger, config.Name, shellscript)
	if err != nil {
		return c, err
	}

	if err := c.Configure(senderManager, config.FastDigest(), instance, config.InitConfig, config.Source); err != nil {
		return c, err
	}

	return c, nil
}
