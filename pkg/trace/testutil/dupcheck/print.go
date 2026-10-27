// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dupcheck

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	// reportWidth is the width past which the stage and partner lists of an
	// issue are wrapped, leaving room for the indentation added by the test
	// log.
	reportWidth = 90
	// maxPrintedValueLen is the length past which printed values are
	// truncated, so that the value line of an example fits in reportWidth.
	maxPrintedValueLen = 66
	// maxPrintedPartners is the number of partners listed for an issue with
	// several partners, the others are counted.
	maxPrintedPartners = 3
)

// Report groups findings into issues, one per location the agent wrote. It
// only prints them: it never fails a test and never writes files.
type Report struct {
	seed int64
	// stages are the stages in the order they were first passed to Add.
	stages      []string
	issues      map[string]*reportIssue
	groups      map[string]*reportGroup
	occurrences int
}

// reportIssue gathers the findings whose subject, the attribute or tag the
// agent wrote, is the same location once list indexes and random corpus keys
// are stripped and the payload format is ignored, whatever their stage, span
// type and partners.
type reportIssue struct {
	subject string
	count   int
	// changes holds the change kinds of the findings by stage.
	changes   map[string]map[string]bool
	partners  map[string]*reportPartner
	spanTypes map[string]bool
	example   Finding
}

// reportPartner is a location an issue's subject repeats.
type reportPartner struct {
	location string
	count    int
	// synonym is set when the partner is a semantic synonym of the subject.
	synonym bool
}

// reportGroup gathers the findings of a fingerprint.
type reportGroup struct {
	count   int
	example Finding
}

// NewReport returns an empty report for a corpus generated with seed.
func NewReport(seed int64) *Report {
	return &Report{seed: seed, issues: make(map[string]*reportIssue), groups: make(map[string]*reportGroup)}
}

// Add records findings produced by stage. The example of an issue or of a
// fingerprint is its smallest finding by context, span ID, value and
// locations, so that it doesn't depend on the order findings are added in.
func (r *Report) Add(stage string, fs ...Finding) {
	if !slices.Contains(r.stages, stage) {
		r.stages = append(r.stages, stage)
	}
	for _, f := range fs {
		f.Stage = stage
		r.occurrences++

		fp := f.Fingerprint()
		g, ok := r.groups[fp]
		if !ok {
			g = &reportGroup{example: f}
			r.groups[fp] = g
		} else if compareExamples(f, g.example) < 0 {
			g.example = f
		}
		g.count++

		locations := issueLocations(f.Locations)
		subject := issueLocation(locations[0])
		is, ok := r.issues[subject]
		if !ok {
			is = &reportIssue{
				subject:   subject,
				changes:   make(map[string]map[string]bool),
				partners:  make(map[string]*reportPartner),
				spanTypes: make(map[string]bool),
				example:   f,
			}
			r.issues[subject] = is
		} else if compareExamples(f, is.example) < 0 {
			is.example = f
		}
		is.count++
		if is.changes[stage] == nil {
			is.changes[stage] = make(map[string]bool)
		}
		is.changes[stage][f.Change] = true
		if f.SpanID != 0 {
			is.spanTypes[f.SpanType] = true
		}
		seen := make(map[string]bool)
		for _, l := range locations[1:] {
			loc := issueLocation(l)
			if seen[loc] {
				continue
			}
			seen[loc] = true
			p, ok := is.partners[loc]
			if !ok {
				p = &reportPartner{location: loc}
				is.partners[loc] = p
			}
			p.count++
			p.synonym = p.synonym || f.Rule == RuleConceptDup
		}
	}
}

// formatNeutralPrefixes rename the attribute maps of the v1 format to the
// names of the v0.4 format, and span attributes of both to span.attr, so that
// an issue covers a location in every format.
var formatNeutralPrefixes = []struct{ from, to string }{
	{"span.meta[", "span.attr["},
	{"span.attributes[", "span.attr["},
	{"chunk.attributes[", "chunk.tags["},
	{"payload.attributes[", "payload.tags["},
}

// issueLocation returns the name of l in the report: normalized, and with
// the same name in every payload format.
func issueLocation(l Location) string {
	s := normalizeLocation(l)
	for _, p := range formatNeutralPrefixes {
		if rest, ok := strings.CutPrefix(s, p.from); ok {
			return p.to + rest
		}
	}
	return s
}

// mergedIssues returns the issues of the report, with each pair of issues
// whose subjects only repeat each other merged into one: which of the two
// locations the agent wrote depends on the input, the duplicate is the same.
// The subject of the merged issue is the one of the issue with the most
// occurrences.
func (r *Report) mergedIssues() []*reportIssue {
	merged := make(map[string]bool)
	var issues []*reportIssue
	for _, subject := range slices.Sorted(maps.Keys(r.issues)) {
		if merged[subject] {
			continue
		}
		a := r.issues[subject]
		b := r.mirror(a)
		if b == nil || merged[b.subject] {
			issues = append(issues, a)
			continue
		}
		merged[a.subject], merged[b.subject] = true, true
		// Keep the issue with the most occurrences, then the first subject.
		if cmp.Or(cmp.Compare(a.count, b.count), cmp.Compare(b.subject, a.subject)) < 0 {
			a, b = b, a
		}
		issues = append(issues, a.mergeMirror(b))
	}
	return issues
}

// mirror returns the issue whose only partner is the subject of is, when the
// only partner of is is the subject of that issue.
func (r *Report) mirror(is *reportIssue) *reportIssue {
	if len(is.partners) != 1 {
		return nil
	}
	for partner := range is.partners {
		other, ok := r.issues[partner]
		if !ok || other == is || len(other.partners) != 1 || other.partners[is.subject] == nil {
			return nil
		}
		return other
	}
	return nil
}

// mergeMirror returns a new issue made of is and its mirror issue other,
// with the subject of is.
func (is *reportIssue) mergeMirror(other *reportIssue) *reportIssue {
	out := &reportIssue{
		subject:   is.subject,
		count:     is.count + other.count,
		changes:   make(map[string]map[string]bool),
		partners:  make(map[string]*reportPartner),
		spanTypes: make(map[string]bool),
		example:   is.example,
	}
	if compareExamples(other.example, out.example) < 0 {
		out.example = other.example
	}
	for _, src := range []*reportIssue{is, other} {
		for stage, changes := range src.changes {
			if out.changes[stage] == nil {
				out.changes[stage] = make(map[string]bool)
			}
			for c := range changes {
				out.changes[stage][c] = true
			}
		}
		for typ := range src.spanTypes {
			out.spanTypes[typ] = true
		}
	}
	p := is.partners[other.subject]
	q := other.partners[is.subject]
	out.partners[other.subject] = &reportPartner{location: other.subject, count: p.count + q.count, synonym: p.synonym || q.synonym}
	return out
}

// issueLocations returns the locations of a finding with its subject first:
// the location the agent touched first, unless it is a top-level field, in
// which case the first attribute it repeats is the subject. That attribute was
// created or changed too, as an attribute left alone is never compared with
// top-level fields.
func issueLocations(locations []Location) []Location {
	if isAttributeLocation(locations[0]) {
		return locations
	}
	for i, l := range locations[1:] {
		if isAttributeLocation(l) {
			out := slices.Clone(locations)
			out[0], out[i+1] = l, locations[0]
			return out
		}
	}
	return locations
}

// isAttributeLocation reports whether l is the location of an attribute, as
// opposed to a top-level field.
func isAttributeLocation(l Location) bool {
	return strings.HasSuffix(string(l), "]")
}

func compareExamples(a, b Finding) int {
	return cmp.Or(
		cmp.Compare(a.Context, b.Context),
		cmp.Compare(a.SpanID, b.SpanID),
		cmp.Compare(a.Value, b.Value),
		slices.Compare(a.Locations, b.Locations),
	)
}

// Print logs the report as a single t.Logf call: a header line, then one
// numbered block per issue, sorted by occurrences then title, with the stages
// that created the duplicate, the partners, the counts and one example. With
// verbose, a second t.Logf call lists every fingerprint with its count and
// one example. It uses t.Logf only, so the output shows with -test.v (and
// --test_output=all under Bazel).
func (r *Report) Print(t testing.TB, verbose bool) {
	t.Helper()
	t.Logf("\n%s", r.render(!verbose))
	if verbose {
		t.Logf("\n%s", r.renderFingerprints())
	}
}

// Summary is the machine-readable form of a report, used to compare the
// reports of two commits.
type Summary struct {
	Seed        int64          `json:"seed"`
	Occurrences int            `json:"occurrences"`
	Issues      []SummaryIssue `json:"issues"`
	// Text is the report as printed by Print, without the verbose part.
	Text string `json:"text"`
}

// SummaryIssue is an issue of a Summary. Subject identifies the issue: its
// title changes when the issue gets a second partner.
type SummaryIssue struct {
	Subject   string         `json:"subject"`
	Title     string         `json:"title"`
	Count     int            `json:"count"`
	CreatedBy []string       `json:"created_by"`
	Partners  map[string]int `json:"partners"`
}

// Summary returns the summary of the report, its issues in report order.
func (r *Report) Summary() Summary {
	s := Summary{Seed: r.seed, Occurrences: r.occurrences, Issues: []SummaryIssue{}, Text: r.render(false)}
	for _, is := range r.sortedIssues() {
		partners := make(map[string]int, len(is.partners))
		for _, p := range is.partners {
			partners[p.location] = p.count
		}
		s.Issues = append(s.Issues, SummaryIssue{
			Subject:   is.subject,
			Title:     is.title(),
			Count:     is.count,
			CreatedBy: is.createdBy(r.stages),
			Partners:  partners,
		})
	}
	return s
}

// sortedIssues returns the merged issues, most frequent first.
func (r *Report) sortedIssues() []*reportIssue {
	issues := r.mergedIssues()
	slices.SortFunc(issues, func(a, b *reportIssue) int {
		return cmp.Or(cmp.Compare(b.count, a.count), cmp.Compare(a.title(), b.title()))
	})
	return issues
}

// render returns the issue report, ending with a hint to use the verbose
// mode when hint is set.
func (r *Report) render(hint bool) string {
	issues := r.sortedIssues()
	var sb strings.Builder
	fmt.Fprintf(&sb, "Trace payload duplicates: %s, %s (seed %d)\n",
		plural(len(issues), "issue"), plural(r.occurrences, "occurrence"), r.seed)
	for i, is := range issues {
		sb.WriteString("\n")
		is.render(&sb, strconv.Itoa(i+1)+". ", r.stages)
	}
	if hint && len(issues) > 0 {
		sb.WriteString("\nRun with -dupcheck.verbose to list every occurrence group.\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// title returns the title of the issue: the subject and the location it
// repeats, or "another location" when it repeats several.
func (is *reportIssue) title() string {
	if len(is.partners) != 1 {
		return is.subject + " repeats another location"
	}
	for _, p := range is.partners {
		title := is.subject + " repeats " + p.location
		if p.synonym {
			title += " (semantic synonyms)"
		}
		return title
	}
	return ""
}

func (is *reportIssue) render(sb *strings.Builder, label string, stageOrder []string) {
	indent := strings.Repeat(" ", len(label))
	sb.WriteString(label + is.title() + "\n")
	sb.WriteString(wrapList(indent+"created by: ", is.createdBy(stageOrder)))
	if len(is.partners) > 1 {
		sb.WriteString(wrapList(indent+"partners: ", is.partnerList()))
	}
	occurrences := indent + "occurrences: " + formatCount(is.count)
	if len(is.spanTypes) > 0 {
		occurrences += " on " + plural(len(is.spanTypes), "span type")
	}
	sb.WriteString(occurrences + "\n")
	e := is.example
	// The span ID is left out of span examples: it says nothing to the reader.
	where := strings.TrimSuffix(e.Context, " span_id="+strconv.FormatUint(e.SpanID, 10))
	sb.WriteString(indent + "example: " + where + "\n")
	sb.WriteString(indent + "         value=" + strconv.Quote(truncate(e.Value)) + "\n")
}

// changeNotes describe the change kinds other than created.
var changeNotes = map[string]string{
	ChangeCreated:     "created",
	ChangeOverwritten: "overwrote existing value",
	ChangeBothChanged: "changed both values",
}

// createdBy lists the stages of the issue in stageOrder, each followed by the
// change kinds of its findings unless they were all created.
func (is *reportIssue) createdBy(stageOrder []string) []string {
	var stages []string
	for _, stage := range stageOrder {
		changes, ok := is.changes[stage]
		if !ok {
			continue
		}
		if len(changes) == 1 && changes[ChangeCreated] {
			stages = append(stages, stage)
			continue
		}
		var notes []string
		for _, c := range []string{ChangeCreated, ChangeOverwritten, ChangeBothChanged} {
			if changes[c] {
				notes = append(notes, changeNotes[c])
			}
		}
		stages = append(stages, stage+" ("+strings.Join(notes, ", ")+")")
	}
	return stages
}

// partnerList returns the most frequent partners with their counts, and how
// many others there are.
func (is *reportIssue) partnerList() []string {
	partners := slices.Collect(maps.Values(is.partners))
	slices.SortFunc(partners, func(a, b *reportPartner) int {
		return cmp.Or(cmp.Compare(b.count, a.count), cmp.Compare(a.location, b.location))
	})
	short := make(map[string]int)
	for _, p := range partners {
		short[shortLocation(p.location)]++
	}
	var items []string
	for _, p := range partners[:min(maxPrintedPartners, len(partners))] {
		name := shortLocation(p.location)
		if short[name] > 1 {
			name = p.location
		}
		note := formatCount(p.count)
		if p.synonym {
			note += ", synonym"
		}
		items = append(items, name+" ("+note+")")
	}
	if more := len(partners) - maxPrintedPartners; more > 0 {
		items = append(items, "+"+formatCount(more)+" more")
	}
	return items
}

// spanFieldNames are the names of the top-level span fields, which a bare
// attribute key must not be confused with.
var spanFieldNames = map[string]bool{
	"service": true, "name": true, "resource": true, "type": true,
	"env": true, "version": true, "component": true, "kind": true,
}

// shortLocation returns the name of a top-level span field and the bare key
// of a span attribute, when it isn't the name of a top-level span field.
// Other locations are returned in full.
func shortLocation(loc string) string {
	for _, prefix := range []string{"span.attr[", "span.meta[", "span.attributes["} {
		if key, ok := strings.CutPrefix(loc, prefix); ok {
			if key, ok := strings.CutSuffix(key, "]"); ok && !spanFieldNames[key] {
				return key
			}
			return loc
		}
	}
	if name, ok := strings.CutPrefix(loc, "span."); ok && spanFieldNames[name] {
		return name
	}
	return loc
}

// wrapList returns items joined by commas after prefix, wrapped at reportWidth
// with the continuation lines aligned after prefix.
func wrapList(prefix string, items []string) string {
	var sb strings.Builder
	line := prefix
	for i, item := range items {
		if i < len(items)-1 {
			item += ","
		}
		switch {
		case i == 0:
			line += item
		case len(line)+1+len(item) > reportWidth:
			sb.WriteString(line + "\n")
			line = strings.Repeat(" ", len(prefix)) + item
		default:
			line += " " + item
		}
	}
	sb.WriteString(line + "\n")
	return sb.String()
}

// renderFingerprints returns a summary line followed by one line per
// fingerprint, sorted, with its count and one example. The fingerprint holds
// the first two locations of a finding, the other locations of the example
// are listed after "also=".
func (r *Report) renderFingerprints() string {
	lines := []string{fmt.Sprintf("dupcheck: %d distinct findings, %d occurrences (seed %d)", len(r.groups), r.occurrences, r.seed)}
	for _, fp := range slices.Sorted(maps.Keys(r.groups)) {
		g := r.groups[fp]
		line := fmt.Sprintf("dupcheck: %s  count=%d  e.g. %s value=%q", fp, g.count, g.example.Context, truncate(g.example.Value))
		if len(g.example.Locations) > 2 {
			also := make([]string, 0, len(g.example.Locations)-2)
			for _, l := range g.example.Locations[2:] {
				also = append(also, string(l))
			}
			line += " also=" + strings.Join(also, ",")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// plural returns n with thousands separators followed by noun, with an "s"
// unless n is 1.
func plural(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return formatCount(n) + " " + noun
}

// formatCount returns the count n with thousands separators.
func formatCount(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func truncate(v string) string {
	runes := []rune(v)
	if len(runes) <= maxPrintedValueLen {
		return v
	}
	return string(runes[:maxPrintedValueLen]) + "..."
}
