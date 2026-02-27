// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package portlist

import "errors"

func (p *Poller) init() {
	p.initErr = errors.New("portlist polling not supported on AIX")
}
