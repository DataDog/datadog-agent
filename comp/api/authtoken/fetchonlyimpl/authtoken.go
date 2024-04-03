// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fetchonlyimpl implements the access to the auth_token used to communicate between Agent
// processes but does not create it.
package fetchonlyimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAuthToken),
		fx.Provide(func(authToken authtoken.Component) optional.Option[authtoken.Component] {
			return optional.NewOption[authtoken.Component](authToken)
		}),
	)
}

type authToken struct {
	log  log.Component
	conf config.Component

	tokenLoaded bool
}

var _ authtoken.Component = (*authToken)(nil)

type dependencies struct {
	fx.In

	Log  log.Component
	Conf config.Component
}

func newAuthToken(deps dependencies) authtoken.Component {
	return &authToken{
		log:  deps.Log,
		conf: deps.Conf,
	}
}

// Get returns the session token
func (at *authToken) Get() string {
	if !at.tokenLoaded {
		// We try to load the auth_token until we succeed since it might be created at some point by another
		// process.
		if err := util.SetAuthToken(at.conf); err != nil {
			at.log.Debugf("could not load auth_token: %s", err)
			return ""
		}
		at.tokenLoaded = true
	}

	return util.GetAuthToken()
}
