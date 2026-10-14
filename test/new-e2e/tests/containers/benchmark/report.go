// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// MarkdownReportFile is the name of the Markdown report, meant to be posted as a pull
	// request comment. It is only written when a metric changed significantly, or couldn't be
	// compared.
	MarkdownReportFile = "report.md"
	// JSONReportFile is the name of the machine-readable report, always written.
	JSONReportFile = "report.json"

	timeFormat = "2006-01-02 15:04 MST"
)

// Unit is the unit of the values of a metric.
type Unit string

const (
	// UnitNanocores is the unit of CPU usage metrics, displayed in millicores.
	UnitNanocores Unit = "nanocores"
	// UnitBytes is the unit of memory metrics, displayed in MiB.
	UnitBytes Unit = "bytes"
	// UnitMilliseconds is the unit of the run times of the checks.
	UnitMilliseconds Unit = "milliseconds"
)

// format formats v, in unit u, in a human-friendly unit, with its sign if signed.
func (u Unit) format(v float64, signed bool) string {
	verb := "%.1f %s"
	if signed {
		verb = "%+.1f %s"
	}
	switch u {
	case UnitNanocores:
		return fmt.Sprintf(verb, v/1e6, "millicores")
	case UnitBytes:
		return fmt.Sprintf(verb, v/(1<<20), "MiB")
	case UnitMilliseconds:
		return fmt.Sprintf(verb, v, "ms")
	}
	return fmt.Sprintf(verb, v, u)
}

// Image is a container image of a component of a variant.
type Image struct {
	Component string `json:"component"`
	Image     string `json:"image"`
	ImageID   string `json:"image_id,omitempty"`
}

// Variant describes the images of one of the two variants of the benchmark.
type Variant struct {
	Name string `json:"name"`
	// Commit is the datadog-agent commit the images were built from, if known.
	Commit string  `json:"commit,omitempty"`
	Images []Image `json:"images"`
	// ProfilesURL lists the profiles of the images over the first phase, during which they
	// run on the installation of the same name.
	ProfilesURL string `json:"profiles_url,omitempty"`
}

// Result is the comparison of a metric of a component between the two variants.
type Result struct {
	Component string `json:"component"`
	Metric    string `json:"metric"`
	// Check is the check whose run time the metric is, if any.
	Check      string     `json:"check,omitempty"`
	Unit       Unit       `json:"unit"`
	Thresholds Thresholds `json:"thresholds"`
	// BaselineQuery and ComparisonQuery are the queries of the baseline and comparison
	// installations, which run the comparison and baseline images in the swapped phase.
	BaselineQuery   string `json:"baseline_query"`
	ComparisonQuery string `json:"comparison_query"`
	ExplorerURL     string `json:"explorer_url,omitempty"`
	Comparison
	Error string `json:"error,omitempty"`
}

// Compare fills the comparison of the result from the points of the images over the first
// and the swapped phases, or records why they can't be compared.
func (r *Result) Compare(first, swapped Phase) {
	c, err := Compare(first, swapped, r.Thresholds)
	if err != nil {
		r.Fail(err)
		return
	}
	r.Comparison = c
}

// metricName returns the name of the metric in Markdown: in code for a metric of Datadog,
// in plain text for the run time of a check.
func (r Result) metricName() string {
	if r.Check != "" {
		return r.Metric
	}
	return "`" + r.Metric + "`"
}

// Fail records that the metric couldn't be compared.
func (r *Result) Fail(err error) {
	r.Comparison = Comparison{Verdict: VerdictError}
	r.Error = err.Error()
}

// Report is the result of the benchmark.
type Report struct {
	ClusterName string `json:"cluster_name"`
	// Phases are the windows of the first phase and of the swapped one.
	Phases     [2]Window `json:"phases"`
	Baseline   Variant   `json:"baseline"`
	Comparison Variant   `json:"comparison"`
	Results    []Result  `json:"results"`
}

// notable reports whether a metric improved, regressed or couldn't be compared: the outcomes
// worth a pull request comment, the last two failing the benchmark.
func (r *Report) notable() bool {
	return slices.ContainsFunc(r.Results, func(result Result) bool {
		return result.Verdict != VerdictUnchanged
	})
}

// Write writes the JSON report to dir, and the Markdown report only if the report is notable.
// Otherwise, it removes the Markdown report of a previous run, if any.
func (r *Report) Write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, JSONReportFile), append(data, '\n'), 0o644); err != nil {
		return err
	}

	markdownPath := filepath.Join(dir, MarkdownReportFile)
	if !r.notable() {
		if err := os.Remove(markdownPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return os.WriteFile(markdownPath, []byte(r.Markdown()), 0o644)
}

// Markdown renders the report in Markdown.
func (r *Report) Markdown() string {
	var b strings.Builder
	// No heading: the pull request comment gets the one of the CI job posting it.
	first, swapped := r.Phases[0], r.Phases[1]
	variants := []Variant{r.Baseline, r.Comparison}
	fmt.Fprintf(&b, "Resource usage of the comparison Agent against the baseline Agent, on the EKS cluster `%s`, in two phases: from %s to %s (%s) with the baseline images on the baseline installation and the comparison images on the comparison one, then from %s to %s (%s) with the images swapped.\n\n",
		r.ClusterName,
		first.From.UTC().Format(timeFormat), first.To.UTC().Format(timeFormat), first.To.Sub(first.From),
		swapped.From.UTC().Format(timeFormat), swapped.To.UTC().Format(timeFormat), swapped.To.Sub(swapped.From))

	b.WriteString("| Component | Metric | Baseline | Comparison | Change | p-value | Verdict |\n")
	b.WriteString("|---|---|--:|--:|--:|--:|---|\n")
	for _, result := range r.Results {
		metric := result.metricName()
		if result.Check != "" {
			metric = "`" + result.Check + "` " + metric
		}
		if result.ExplorerURL != "" {
			metric = "[" + metric + "](" + result.ExplorerURL + ")"
		}
		if result.Error != "" {
			fmt.Fprintf(&b, "| %s | %s | | | | | **error**: %s |\n", result.Component, metric, markdownCell(result.Error))
			continue
		}
		verdict := string(result.Verdict)
		if result.Verdict != VerdictUnchanged {
			verdict = "**" + verdict + "**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %+.1f%% (%s) | %s | %s |\n",
			result.Component, metric,
			result.Unit.format(result.BaselineMean, false),
			result.Unit.format(result.ComparisonMean, false),
			100*result.RelativeChange, result.Unit.format(result.ComparisonMean-result.BaselineMean, true),
			formatPValue(result.PValue), verdict)
	}

	var metrics, thresholds []string
	significance := SignificanceLevel
	for _, result := range r.Results {
		significance = result.Thresholds.significance()
		if slices.Contains(metrics, result.Metric) {
			continue
		}
		metrics = append(metrics, result.Metric)
		thresholds = append(thresholds, fmt.Sprintf("%s %.0f%% and %s", result.metricName(), 100*result.Thresholds.Relative, result.Unit.format(result.Thresholds.Absolute, false)))
	}
	var runTimeBlocks string
	if slices.ContainsFunc(r.Results, func(result Result) bool { return result.Check != "" }) {
		runTimeBlocks = fmt.Sprintf(" (%s for the check run times, measured run by run, hence much less autocorrelated)", RunTimeWindow(first).Block)
	}
	fmt.Fprintf(&b, "\nThe installations run on node pools whose hardware may differ, hence the swap. "+
		"Each phase is split into blocks of %s%s, in which the means of the images are compared through their log ratio. "+
		"The change is exp(e) - 1, e being the average over both phases of the mean log ratio of each phase, in which the difference between the node pools cancels out. "+
		"Baseline and Comparison are the averages of the means of each image over both phases. "+
		"A change is an improvement or a regression when it is significant (t-test of e = 0, %s) and at least as large as both thresholds of its metric: %s.\n\n",
		first.Block, runTimeBlocks, formatSignificance(significance), strings.Join(thresholds, "; "))

	for _, variant := range variants {
		if variant.Commit != "" {
			fmt.Fprintf(&b, "- %s commit: [`%.8s`](https://github.com/DataDog/datadog-agent/commit/%s)\n", variant.Name, variant.Commit, variant.Commit)
		}
	}
	b.WriteString("\n| Variant | Component | Image |\n")
	b.WriteString("|---|---|---|\n")
	for _, variant := range variants {
		for _, image := range variant.Images {
			reference := "`" + image.Image + "`"
			if image.ImageID != "" {
				reference += " (`" + image.ImageID + "`)"
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", variant.Name, image.Component, reference)
		}
	}

	var profiles []string
	for _, variant := range variants {
		if variant.ProfilesURL != "" {
			profiles = append(profiles, "["+variant.Name+"]("+variant.ProfilesURL+")")
		}
	}
	if len(profiles) > 0 {
		b.WriteString("\nProfiles of the first phase: " + strings.Join(profiles, ", ") + "\n")
	}
	return b.String()
}

// formatSignificance describes the p-value under which a change is significant.
func formatSignificance(significance float64) string {
	if significance < SignificanceLevel {
		return fmt.Sprintf("p < %.2g, i.e. %g divided by the number of compared metrics", significance, SignificanceLevel)
	}
	return fmt.Sprintf("p < %g", significance)
}

func formatPValue(p float64) string {
	if p < 0.001 {
		return "< 0.001"
	}
	return strconv.FormatFloat(p, 'f', 3, 64)
}

// markdownCell escapes text to fit in a cell of a Markdown table.
func markdownCell(text string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(text)
}

// MetricsExplorerURL returns the URL of the metrics explorer of the Datadog app at appURL
// graphing the average of metric over [from, to], filtered by scope and split by groupBy.
func MetricsExplorerURL(appURL, metric string, scope, groupBy []string, from, to time.Time) string {
	values := url.Values{}
	values.Set("exp_metric", metric)
	values.Set("exp_scope", strings.Join(scope, ","))
	values.Set("exp_group", strings.Join(groupBy, ","))
	values.Set("exp_agg", "avg")
	values.Set("exp_row_type", "metric")
	setTimeRange(values, from, to)
	return appURL + "/metric/explorer?" + values.Encode()
}

// ProfilesURL returns the URL of the profiling explorer of the Datadog app at appURL listing
// the profiles matching query over [from, to].
func ProfilesURL(appURL, query string, from, to time.Time) string {
	values := url.Values{}
	values.Set("query", query)
	setTimeRange(values, from, to)
	return appURL + "/profiling/explorer?" + values.Encode()
}

func setTimeRange(values url.Values, from, to time.Time) {
	values.Set("from_ts", strconv.FormatInt(from.UnixMilli(), 10))
	values.Set("to_ts", strconv.FormatInt(to.UnixMilli(), 10))
	values.Set("live", "false")
}
