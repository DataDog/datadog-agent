package procdiscovery

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/fatih/color"
)

func (d DiscoveredIntegrations) Render() string {
	var buf bytes.Buffer

	if len(d.Running) != 0 {
		fmt.Fprintln(&buf, "Running checks:")
		for integration := range d.Running {
			fmt.Fprintln(&buf, fmt.Sprintf("\t- %s", color.YellowString(integration)))
		}
		fmt.Fprintln(&buf)
	}

	if len(d.Failing) != 0 {
		fmt.Fprintln(&buf, "Failing checks:")
		for integration := range d.Failing {
			fmt.Fprintln(&buf, fmt.Sprintf("\t- %s", color.RedString(integration)))
		}
		fmt.Fprintln(&buf)
	}

	if len(d.Discovered) == 0 {
		fmt.Fprintln(&buf, "There was no new integration found.")
		return buf.String()
	}

	for integration, processes := range d.Discovered {
		_, isRunning := d.Running[integration]
		_, isFailing := d.Failing[integration]

		// Do not show integrations that are already configured
		if !(isRunning || isFailing) {
			header := "Discovered '%s' for processes:"
			if len(processes) == 1 {
				header = "Discovered '%s' for process:"
			}

			fmt.Fprintln(&buf, fmt.Sprintf(header, color.GreenString(integration)))
			for _, proc := range processes {
				fmt.Fprintln(&buf, fmt.Sprintf("\t- %s", prettifyCmd(proc.Cmd)))
			}
			fmt.Fprintln(&buf)
		}
	}

	return buf.String()
}

func prettifyCmd(cmd string) string {
	fields := strings.Fields(cmd)

	if len(fields) == 0 {
		return ""
	}

	fields[0] = color.BlueString(fields[0])

	for i := 0; i < len(fields); i++ {
		// CLI option
		if strings.HasPrefix(fields[i], "-") {
			fields[i] = color.CyanString(fields[i])
		}
	}

	return strings.Join(fields, " ")
}
