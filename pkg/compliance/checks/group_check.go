// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var groupReportedFields = []string{
	compliance.GroupFieldName,
	compliance.GroupFieldUsers,
	compliance.GroupFieldID,
}

// ErrGroupNotFound is returned when a group cannot be found
var ErrGroupNotFound = errors.New("group not found")

func resolveGroup(_ context.Context, e env.Env, id string, res compliance.BaseResource) (resolved, error) {
	if res.Group == nil {
		return nil, fmt.Errorf("%s: expecting group resource in group check", id)
	}

	group := res.Group

	f, err := os.Open(e.EtcGroupPath())
	if err != nil {
		log.Errorf("%s: failed to open %s: %v", id, e.EtcGroupPath(), err)
		return nil, err
	}

	defer f.Close()

	finder := &groupFinder{
		groupName: group.Name,
	}

	err = readEtcGroup(f, finder.findGroup)
	if err != nil {
		return nil, wrapErrorWithID(id, err)
	}

	if finder.instance == nil {
		return nil, ErrGroupNotFound
	}

	return newResolvedInstance(finder.instance, group.Name, "group"), nil
}

type groupFinder struct {
	groupName string
	instance  eval.Instance
}

func (f *groupFinder) findGroup(line []byte) (bool, error) {
	substr := []byte(f.groupName + ":")
	if !bytes.HasPrefix(line, substr) {
		return false, nil
	}

	const expectParts = 4
	parts := strings.SplitN(string(line), ":", expectParts)

	if len(parts) != expectParts {
		log.Errorf("malformed line in group file - expected %d, found %d segments", expectParts, len(parts))
		return false, errors.New("malformed group file format")
	}

	gid, err := strconv.Atoi(parts[2])
	if err != nil {
		log.Errorf("failed to parse group ID for %s: %v", f.groupName, err)
	}

	f.instance = eval.NewInstance(
		eval.VarMap{
			compliance.GroupFieldName:  f.groupName,
			compliance.GroupFieldUsers: strings.Split(parts[3], ","),
			compliance.GroupFieldID:    gid,
		},
		nil,
	)

	return true, nil
}

type lineFunc func(line []byte) (bool, error)

// Copyright (c) 2009 The Go Authors. All rights reserved.
// Adapted from Go readGroup code

func readEtcGroup(r io.Reader, fn lineFunc) error {
	bs := bufio.NewScanner(r)
	for bs.Scan() {
		line := bs.Bytes()
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		done, err := fn(line)
		if done || err != nil {
			return err
		}
	}
	return bs.Err()
}
