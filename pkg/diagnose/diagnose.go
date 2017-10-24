package diagnose

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/fatih/color"
)

// Diagnose runs some connectivity checks, output it in writer
func Diagnose(w io.Writer) {
	if w != os.Stdout {
		color.NoColor = true
	}

	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	fmt.Fprintln(tw, "Diagnosis \t")
	for name, d := range diagnosis.DefaultCatalog {
		err := d.Diagnose()
		statusString := color.GreenString("PASS")
		if err != nil {
			statusString = color.RedString("FAIL")
		}
		fmt.Fprintln(tw, fmt.Sprintf("%s \t%s \t", name, statusString))
	}
	tw.Flush()
}
