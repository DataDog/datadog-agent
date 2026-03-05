// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).

package diagnose

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	dproxy "github.com/DataDog/datadog-agent/pkg/diagnose/proxy"
	"github.com/spf13/cobra"
)

func newProxyCommand() *cobra.Command {
	var jsonOut bool
	var noNetwork bool
	var summaryOut bool
	var includeSensitive bool // reserved (for probe/TLS chain output later)

	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Diagnose proxy/TLS configuration and common pitfalls",
		Long:  "Shows effective proxy settings with source precedence and lints common no_proxy and conflict issues.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = includeSensitive // not used in MVP

			// --- IMPORTANT: honor -c/--cfgpath by exporting DD_CONF_DIR ---
			// Our config reader (pkg/config/setup) already respects DD_CONF_DIR.
			// The diagnose root may not preload config in --local/-o mode, so
			// we set the env ourselves before calling into the proxy code.
			if f := cmd.InheritedFlags().Lookup("cfgpath"); f != nil {
				if v := f.Value.String(); v != "" {
					_ = os.Setenv("DD_CONF_DIR", v)
				}
			}
			// --------------------------------------------------------------

			res := dproxy.Run(noNetwork)

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}

			if summaryOut {
				fmt.Print(dproxy.FormatSummary(res))
				return nil
			}
			fmt.Printf("Proxy/TLS Diagnose: %s\n\n", res.Summary)
			fmt.Printf("Effective proxy (with sources):\n")
			fmt.Printf("  HTTPS    : %q [%s]\n", dproxy.RedactURL(res.Effective.HTTPS.Value), res.Effective.HTTPS.Source)
			fmt.Printf("  HTTP     : %q [%s]\n", dproxy.RedactURL(res.Effective.HTTP.Value), res.Effective.HTTP.Source)
			fmt.Printf("  NO_PROXY : %q [%s]\n\n", res.Effective.NoProxy.Value, res.Effective.NoProxy.Source)
			fmt.Println("NO_PROXY evaluation:")
			if len(res.EndpointMatrix) == 0 {
				fmt.Println("  (no endpoints discovered)")
			} else {
				for _, row := range res.EndpointMatrix {
					b := "no"
					if row.Bypassed {
						b = "YES"
					}
					fmt.Printf("  - %-12s host=%s port=%s bypassed=%s token=%q\n",
						row.Endpoint.Name, row.Host, row.Port, b, row.Matched)
				}
			}
			fmt.Println()

			if len(res.Findings) == 0 {
				fmt.Println("Findings: none. Looks good ✅")
				return nil
			}
			fmt.Println("Findings:")
			for _, f := range res.Findings {
				fmt.Printf("  - [%s] %s\n    → %s\n", f.Severity, f.Description, f.Action)
			}
			return nil
		},
	}

	// Flags
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&noNetwork, "no-network", true, "Do not run active network probes. Set to false to run DNS/TCP/TLS/HTTP checks.")
	cmd.Flags().BoolVar(&summaryOut, "summary", false, "Print a compact, privacy-safe summary block.")
	cmd.Flags().BoolVar(&includeSensitive, "include-sensitive", false, "Include sensitive details (reserved)")

	return cmd
}

func writeProxySummary(res dproxy.Result) {
	// header
	utc := time.Now().UTC().Format(time.RFC3339)
	fmt.Printf("=== Proxy/TLS Diagnose (privacy-safe, %s) ===\n", utc)
	fmt.Printf("Summary: %s\n", res.Summary)

	// effective (scrubbed via RedactURL in CLI; JSON already redacts via MarshalJSON)
	fmt.Printf("HTTPS: %q [%s]\n", dproxy.RedactURL(res.Effective.HTTPS.Value), res.Effective.HTTPS.Source)
	fmt.Printf("HTTP : %q [%s]\n", dproxy.RedactURL(res.Effective.HTTP.Value), res.Effective.HTTP.Source)
	if res.Effective.NoProxy.Value != "" {
		fmt.Printf("NO_PROXY: %q [%s]\n", res.Effective.NoProxy.Value, res.Effective.NoProxy.Source)
	}

	// key findings (keep it short)
	printed := 0
	for _, f := range res.Findings {
		if printed >= 6 {
			fmt.Println("…")
			break
		}
		fmt.Printf("- [%s] %s\n", f.Severity, f.Description)
		printed++
	}

	// endpoint bypass hint
	byp := 0
	for _, row := range res.EndpointMatrix {
		if row.Bypassed {
			byp++
		}
	}
	if byp > 0 {
		fmt.Printf("Bypass: %d intake(s) bypass proxy due to NO_PROXY.\n", byp)
	}
	fmt.Println()
}
