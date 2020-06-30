// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type groupCheck struct {
	baseCheck
	etcGroupPath string
	group        *compliance.Group
}

func newGroupCheck(baseCheck baseCheck, etcGroupPath string, group *compliance.Group) (*groupCheck, error) {
	return &groupCheck{
		baseCheck:    baseCheck,
		etcGroupPath: etcGroupPath,
		group:        group,
	}, nil
}

func (c *groupCheck) Run() error {
	f, err := os.Open(c.etcGroupPath)

	if err != nil {
		log.Errorf("%s: failed to open %s: %v", c.id, c.etcGroupPath, err)
		return err
	}

	defer f.Close()

	return readEtcGroup(f, c.findGroup)
}

func (c *groupCheck) findGroup(line []byte) (bool, error) {
	substr := []byte(c.group.Name + ":")
	if !bytes.HasPrefix(line, substr) {
		return false, nil
	}

	const expectParts = 4
	parts := strings.SplitN(string(line), ":", expectParts)

	if len(parts) != expectParts {
		log.Errorf("%s: malformed line in group file - expected %d, found %d segments", c.id, expectParts, len(parts))
		return false, errors.New("malformed group file format")
	}

	kv := compliance.KVMap{}
	for _, field := range c.group.Report {

		if c.setStaticKV(field, kv) {
			continue
		}

		var v string
		switch field.Kind {
		case compliance.PropertyKindAttribute:
			switch field.Property {
			case "users", "members":
				v = parts[3]
			case "name":
				v = parts[1]
			case "group_id", "gid":
				v = parts[2]
			default:
				return false, ErrPropertyNotSupported
			}
		default:
			return false, ErrPropertyKindNotSupported
		}

		key := field.As
		if key == "" {
			key = field.Property
		}

		if v != "" {
			kv[key] = v
		}
	}

	c.report(nil, kv)
	return true, nil
}

type lineFunc func(line []byte) (bool, error)

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
