// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// RegisterFlags registers one pflag per schema field on fs.
func RegisterFlags(s Schema, fs *pflag.FlagSet) {
	for _, f := range s.Fields {
		help := f.Help
		if len(f.Enum) > 0 {
			help = strings.TrimSpace(help + " (one of: " + strings.Join(f.Enum, ", ") + ")")
		}
		switch f.Kind {
		case KindBool:
			fs.Bool(f.Name, f.Default == "true", help)
		case KindInt:
			def := 0
			// best-effort default; Decode re-applies the canonical default too.
			if f.Default != "" {
				_, _ = fmtSscan(f.Default, &def)
			}
			fs.Int(f.Name, def, help)
		default:
			fs.String(f.Name, f.Default, help)
		}
	}
}

// CollectFlags returns only flags the user explicitly changed, stringified.
func CollectFlags(s Schema, fs *pflag.FlagSet) map[string]string {
	out := map[string]string{}
	for _, f := range s.Fields {
		fl := fs.Lookup(f.Name)
		if fl == nil || !fl.Changed {
			continue
		}
		out[f.Name] = fl.Value.String()
	}
	return out
}

// fmtSscan is a small helper that keeps RegisterFlags readable by
// isolating the fmt import usage for integer default parsing.
func fmtSscan(s string, p *int) (int, error) { return fmt.Sscan(s, p) }
