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

	"github.com/samber/lo"
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

// Image is a container image of a component of a version.
type Image struct {
	Component string `json:"component"`
	Image     string `json:"image"`
	ImageID   string `json:"image_id,omitempty"`
}

// Version describes the images of one of the two versions of the Agent the benchmark
// compares.
type Version struct {
	Name string `json:"name"`
	// Commit is the datadog-agent commit the images were built from, if known.
	Commit string  `json:"commit,omitempty"`
	Images []Image `json:"images"`
	// ProfilesURL lists the profiles of the images over the first phase.
	ProfilesURL string `json:"profiles_url,omitempty"`
}

// Result is the comparison of a metric of a component between the two versions.
type Result struct {
	Component string `json:"component"`
	Metric    string `json:"metric"`
	// Check is the check whose run time the metric is, if any.
	Check      string     `json:"check,omitempty"`
	Unit       Unit       `json:"unit"`
	Thresholds Thresholds `json:"thresholds"`
	// QueryA and QueryB are the queries of the installations of variants a and b, which
	// run the baseline and comparison images in the first phase, and swap them between
	// consecutive phases (see SwapInPhase).
	QueryA      string `json:"query_a"`
	QueryB      string `json:"query_b"`
	ExplorerURL string `json:"explorer_url,omitempty"`
	Comparison
	Error string `json:"error,omitempty"`
}

// Compare fills the comparison of the result from the points of the images over the phases
// of the measurement, or records why they can't be compared.
func (r *Result) Compare(phases []Phase) {
	c, err := Compare(phases, r.Thresholds)
	if err != nil {
		r.Fail(err)
		return
	}
	r.Comparison, r.Error = c, ""
}

// Name returns the name of the compared metric, prefixed with the check for the run time of
// a check.
func (r Result) Name() string {
	if r.Check != "" {
		return r.Check + " " + r.Metric
	}
	return r.Metric
}

// metricName returns the Name of the metric in Markdown: in code for a metric of Datadog,
// in plain text after the check in code for the run time of a check.
func (r Result) metricName() string {
	if r.Check != "" {
		return "`" + r.Check + "` " + r.Metric
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
	// Phases are the windows of the phases of the measurement, in order (see SwapInPhase for
	// the images each installation runs in each of them).
	Phases       []Window `json:"phases"`
	Baseline     Version  `json:"baseline"`
	Comparison   Version  `json:"comparison"`
	Results      []Result `json:"results"`
	DashboardURL string   `json:"dashboard_url,omitempty"`
}

// notable reports whether a metric improved, regressed or couldn't be compared: the outcomes
// worth a pull request comment, the last two failing the benchmark.
func (r *Report) notable() bool {
	return slices.ContainsFunc(r.Results, func(result Result) bool {
		return result.Verdict != VerdictUnchanged
	})
}

// significance returns the p-value under which the changes of the results are significant,
// and whether it is the same for all of them: if not, the p-value is that of the first result.
func (r *Report) significance() (float64, bool) {
	if len(r.Results) == 0 {
		return SignificanceLevel, true
	}
	significance := r.Results[0].Thresholds.significance()
	return significance, lo.EveryBy(r.Results[1:], func(result Result) bool {
		return result.Thresholds.significance() == significance
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
	// No heading: whoever publishes the report, e.g. as a pull request comment, adds its own.
	versions := []Version{r.Baseline, r.Comparison}
	fmt.Fprintf(&b, "Resource usage of the comparison Agent against the baseline Agent, on the EKS cluster `%s`, in %d phases, between which installations a and b, each with node pools of its own, swap their images:\n\n",
		r.ClusterName, len(r.Phases))
	for i, phase := range r.Phases {
		baselineOn, comparisonOn := SwapInPhase(i, "a", "b")
		fmt.Fprintf(&b, "- phase %d, from %s to %s (%s): baseline images on installation %s, comparison images on installation %s\n", i+1,
			phase.From.UTC().Format(timeFormat), phase.To.UTC().Format(timeFormat), phase.To.Sub(phase.From), baselineOn, comparisonOn)
	}
	b.WriteString("\n")
	if r.DashboardURL != "" {
		fmt.Fprintf(&b, "Graphs and profiles of the run: [dashboard](%s)\n\n", r.DashboardURL)
	}

	b.WriteString("| Component | Metric | Baseline | Comparison | Change | p-value | Verdict |\n")
	b.WriteString("|---|---|--:|--:|--:|--:|---|\n")
	for _, result := range r.Results {
		metric := result.metricName()
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

	// The significance level is stated once if it is the same for all the results, with the
	// thresholds of each metric otherwise.
	significance, uniform := r.significance()
	// The thresholds of the run times differ by check, and the same metric is measured on
	// several components: identical thresholds are listed once. lo.UniqMap would lose their
	// order.
	thresholds := lo.Uniq(lo.Map(r.Results, func(result Result, _ int) string {
		threshold := fmt.Sprintf("%s %.0f%% and %s", result.metricName(), 100*result.Thresholds.Relative, result.Unit.format(result.Thresholds.Absolute, false))
		if !uniform {
			threshold += fmt.Sprintf(" (p < %.2g)", result.Thresholds.significance())
		}
		return threshold
	}))
	significanceText := "p below the level given with the thresholds of its metric"
	if uniform {
		significanceText = formatSignificance(significance)
	}
	fmt.Fprintf(&b, "\nThe machines of the node pools of the installations may differ, hence the swaps: each image runs half of the phases on the node pools of each installation. "+
		"Each phase is split into blocks of %s, in which the means of the images are compared through their log ratio. "+
		"The change is exp(e) - 1, e being the average over the phases of the mean log ratio of each phase, in which the difference between the node pools cancels out. "+
		"Baseline and Comparison are the averages of the means of each image over the phases. "+
		"A change is an improvement or a regression when it is significant (t-test of e = 0, %s) and at least as large as both thresholds of its metric: %s.\n\n",
		// All the phases have blocks of the same duration.
		lo.FirstOrEmpty(r.Phases).Block, significanceText, strings.Join(thresholds, "; "))

	for _, version := range versions {
		if version.Commit != "" {
			fmt.Fprintf(&b, "- %s commit: [`%.8s`](https://github.com/DataDog/datadog-agent/commit/%s)\n", version.Name, version.Commit, version.Commit)
		}
	}
	b.WriteString("\n| Version | Component | Image |\n")
	b.WriteString("|---|---|---|\n")
	for _, version := range versions {
		for _, image := range version.Images {
			reference := "`" + image.Image + "`"
			if image.ImageID != "" {
				reference += " (`" + image.ImageID + "`)"
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", version.Name, image.Component, reference)
		}
	}

	profiles := lo.FilterMap(versions, func(version Version, _ int) (string, bool) {
		return "[" + version.Name + "](" + version.ProfilesURL + ")", version.ProfilesURL != ""
	})
	if len(profiles) > 0 {
		fmt.Fprintf(&b, "\nProfiles of the first phase: %s\n", strings.Join(profiles, ", "))
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

// MetricsExplorerURL returns the URL of the metrics explorer of the Datadog app at the root of
// appURL graphing the average of metric over [from, to], filtered by scope and split by groupBy.
func MetricsExplorerURL(appURL url.URL, metric string, scope, groupBy []string, from, to time.Time) string {
	values := url.Values{}
	values.Set("exp_metric", metric)
	values.Set("exp_scope", strings.Join(scope, ","))
	values.Set("exp_group", strings.Join(groupBy, ","))
	values.Set("exp_agg", "avg")
	values.Set("exp_row_type", "metric")
	return appPageURL(appURL, "/metric/explorer", values, from, to)
}

// ProfilesURL returns the URL of the profiling explorer of the Datadog app at the root of appURL
// listing the profiles matching query over [from, to].
func ProfilesURL(appURL url.URL, query string, from, to time.Time) string {
	values := url.Values{}
	values.Set("query", query)
	return appPageURL(appURL, "/profiling/explorer", values, from, to)
}

// DashboardURL returns the URL of the dashboard dashboardID of the Datadog app at the root of
// appURL over [from, to], with its template variables set to templateVariables, by name.
func DashboardURL(appURL url.URL, dashboardID string, templateVariables map[string]string, from, to time.Time) string {
	values := url.Values{}
	for name, value := range templateVariables {
		values.Set("tpl_var_"+name+"[0]", value)
	}
	values.Set("refresh_mode", "paused")
	return appPageURL(appURL, "/dashboard/"+dashboardID, values, from, to)
}

// appPageURL returns the URL of the page at path of the Datadog app at appURL, with the query
// values, to which it adds the time range [from, to]. The app is at the root of appURL: only
// its scheme and host are kept, and path, escaped as needed, replaces its path.
func appPageURL(appURL url.URL, path string, values url.Values, from, to time.Time) string {
	values.Set("from_ts", strconv.FormatInt(from.UnixMilli(), 10))
	values.Set("to_ts", strconv.FormatInt(to.UnixMilli(), 10))
	values.Set("live", "false")
	page := url.URL{Scheme: appURL.Scheme, Host: appURL.Host, Path: path, RawQuery: values.Encode()}
	return page.String()
}
