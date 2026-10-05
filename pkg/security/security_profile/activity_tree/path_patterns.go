// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils/pathutils"
)

// PathPatternConfig controls sibling pattern mining on FileNode maps.
// Mining is opt-in per ActivityTree via Stats.SetPathPatternConfig; v2
// security profiles enable it, v1 profiles and activity dumps leave it
// off.
type PathPatternConfig struct {
	Enabled                  bool
	MaxChildren              int
	MinClusterSize           int
	MinClusterSizeOnFinalize int
}

// DefaultPathPatternConfig returns the enabled configuration used by v2
// security profiles.
func DefaultPathPatternConfig() PathPatternConfig {
	return PathPatternConfig{
		Enabled:                  true,
		MaxChildren:              15,
		MinClusterSize:           5,
		MinClusterSizeOnFinalize: 3,
	}
}

// tokenClass is the character class of one token of a file name.
// classLiteral tokens (separators, pieces with unusual characters) never
// generalize; every other class has a typed placeholder.
type tokenClass uint8

const (
	classLiteral tokenClass = iota
	classNum
	classHex
	classUUID
	classAlpha
	classAlnum
)

// minHexLen is the shortest piece classified as a hex identifier, so
// short words made of a-f letters ("cafe", "bad") stay alpha.
const minHexLen = 8

type classInfo struct {
	code        string
	placeholder string
	regex       string
	// width ranks how much a placeholder accepts; lower is narrower.
	width int
}

var classes = [...]classInfo{
	classNum:   {code: "N", placeholder: "<num>", regex: `[0-9]+`, width: 1},
	classUUID:  {code: "U", placeholder: "<uuid>", regex: `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`, width: 1},
	classHex:   {code: "H", placeholder: "<hex>", regex: `[0-9a-fA-F]{8,}`, width: 2},
	classAlpha: {code: "A", placeholder: "<alpha>", regex: `[A-Za-z]+`, width: 3},
	classAlnum: {code: "M", placeholder: "<alnum>", regex: `[0-9A-Za-z]*(?:[0-9][A-Za-z]|[A-Za-z][0-9])[0-9A-Za-z]*`, width: 4},
}

var placeholderClasses = map[string]tokenClass{
	classes[classNum].placeholder:   classNum,
	classes[classUUID].placeholder:  classUUID,
	classes[classHex].placeholder:   classHex,
	classes[classAlpha].placeholder: classAlpha,
	classes[classAlnum].placeholder: classAlnum,
}

type nameToken struct {
	text  string
	class tokenClass
}

// tokenizeName splits name into separator, UUID and piece tokens.
func tokenizeName(name string) []nameToken {
	var out []nameToken
	i := 0
	for i < len(name) {
		if isSeparator(name[i]) {
			out = append(out, nameToken{text: name[i : i+1], class: classLiteral})
			i++
			continue
		}
		if isUUIDAt(name, i) {
			out = append(out, nameToken{text: name[i : i+36], class: classUUID})
			i += 36
			continue
		}
		j := i
		for j < len(name) && !isSeparator(name[j]) {
			j++
		}
		out = appendPieceTokens(out, name[i:j])
		i = j
	}
	return out
}

func appendPieceTokens(out []nameToken, piece string) []nameToken {
	hasAlpha, hasDigit, allHex := false, false, true
	for i := 0; i < len(piece); i++ {
		c := piece[i]
		switch {
		case isDigit(c):
			hasDigit = true
		case isAlpha(c):
			hasAlpha = true
			if !isHexLetter(c) {
				allHex = false
			}
		default:
			return append(out, nameToken{text: piece, class: classLiteral})
		}
	}
	switch {
	case !hasAlpha:
		return append(out, nameToken{text: piece, class: classNum})
	case allHex && len(piece) >= minHexLen:
		return append(out, nameToken{text: piece, class: classHex})
	case !hasDigit:
		return append(out, nameToken{text: piece, class: classAlpha})
	}
	return append(out, nameToken{text: piece, class: classAlnum})
}

// leadingLetters returns the run of letters at the start of s.
func leadingLetters(s string) string {
	i := 0
	for i < len(s) && isAlpha(s[i]) {
		i++
	}
	return s[:i]
}

// trailingLetters returns the run of letters at the end of s.
func trailingLetters(s string) string {
	i := len(s)
	for i > 0 && isAlpha(s[i-1]) {
		i--
	}
	return s[i:]
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return s != ""
}

// isUUIDAt reports whether a canonical 8-4-4-4-12 UUID starts at name[i]
// and ends at a separator or the end of name.
func isUUIDAt(name string, i int) bool {
	const uuidLen = 36
	if len(name)-i < uuidLen {
		return false
	}
	if end := i + uuidLen; end < len(name) && !isSeparator(name[end]) {
		return false
	}
	for k := 0; k < uuidLen; k++ {
		c := name[i+k]
		switch k {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexChar(c) {
				return false
			}
		}
	}
	return true
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isAlpha(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isHexLetter(c byte) bool {
	return ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

func isSeparator(c byte) bool {
	return c == '-' || c == '.' || c == '_'
}

// structureSignature returns a canonical skeleton of name: separators
// kept literally, one class code per token, and unusual pieces quoted
// with NUL (which cannot appear in a file name) so they never collide.
// Examples: "sess-42" -> "A-N", "sess42" -> "M", "2024-01-15.log" -> "N-N-N.A".
func structureSignature(name string) string {
	var out strings.Builder
	for _, tok := range tokenizeName(name) {
		if tok.class != classLiteral {
			out.WriteString(classes[tok.class].code)
		} else if len(tok.text) == 1 && isSeparator(tok.text[0]) {
			out.WriteString(tok.text)
		} else {
			out.WriteByte(0)
			out.WriteString(tok.text)
			out.WriteByte(0)
		}
	}
	return out.String()
}

// buildTemplate returns the merged name for a cluster of same-signature
// siblings, keeping tokens on which all siblings agree and replacing the
// others with the placeholder of their class. Mixed letter/digit tokens
// keep the letter prefix and suffix shared by all siblings.
// Examples: [sess-1, sess-22] -> "sess-<num>", [sess1, sess22] -> "sess<num>".
func buildTemplate(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	tokenized := make([][]nameToken, len(names))
	for i, n := range names {
		tokenized[i] = tokenizeName(n)
	}
	ref := tokenized[0]
	var out strings.Builder
	texts := make([]string, len(names))
	for k, tok := range ref {
		allSame := true
		for i, toks := range tokenized {
			if k >= len(toks) {
				allSame = false
				texts[i] = ""
				continue
			}
			texts[i] = toks[k].text
			if texts[i] != tok.text {
				allSame = false
			}
		}
		switch {
		case allSame || tok.class == classLiteral:
			out.WriteString(tok.text)
		case tok.class == classAlnum:
			out.WriteString(alnumTemplate(texts))
		default:
			out.WriteString(classes[tok.class].placeholder)
		}
	}
	return out.String()
}

// alnumTemplate builds the template of one mixed letter/digit position:
// the letter prefix and suffix shared by all texts are kept, and the rest
// becomes <num> when it is digits only for every text, <alnum> otherwise.
// Examples: [sess42, sess7] -> "sess<num>", [a7k2x9, q9w8e7] -> "<alnum>".
func alnumTemplate(texts []string) string {
	prefix := leadingLetters(texts[0])
	suffix := trailingLetters(texts[0])
	for _, t := range texts[1:] {
		if leadingLetters(t) != prefix {
			prefix = ""
		}
		if trailingLetters(t) != suffix {
			suffix = ""
		}
	}
	class := classNum
	for _, t := range texts {
		if !allDigits(t[len(prefix) : len(t)-len(suffix)]) {
			class = classAlnum
			break
		}
	}
	return prefix + classes[class].placeholder + suffix
}

// affixKey identifies the letter prefix and suffix of every mixed
// letter/digit token of name, so that families such as sess1, sess2 are
// split from unrelated random IDs sharing the same signature.
func affixKey(name string) string {
	var key strings.Builder
	for _, tok := range tokenizeName(name) {
		if tok.class != classAlnum {
			continue
		}
		key.WriteString(leadingLetters(tok.text))
		key.WriteByte(0)
		key.WriteString(trailingLetters(tok.text))
		key.WriteByte(0)
	}
	return key.String()
}

// hasSharedLetters reports whether an affixKey carries a letter prefix or
// suffix. Keys of names without mixed tokens are empty and count as shared.
func hasSharedLetters(key string) bool {
	return key == "" || strings.Trim(key, "\x00") != ""
}

// Thresholds of a random-looking mixed token: generated IDs are long and
// either mix case (5nVjCHLMxA) or alternate letters and digits (4fo3s6f9),
// unlike real names such as utf8, gb2312, urllib3 or EST5EDT.
const (
	minRandomTokenLen  = 6
	minRandomTokenRuns = 4
)

// letterDigitRuns counts the alternating letter and digit runs of s.
func letterDigitRuns(s string) int {
	runs := 0
	for i := 0; i < len(s); i++ {
		if i == 0 || isDigit(s[i]) != isDigit(s[i-1]) {
			runs++
		}
	}
	return runs
}

func hasMixedCase(s string) bool {
	lower, upper := false, false
	for i := 0; i < len(s); i++ {
		lower = lower || ('a' <= s[i] && s[i] <= 'z')
		upper = upper || ('A' <= s[i] && s[i] <= 'Z')
	}
	return lower && upper
}

// looksRandom reports whether name holds a mixed letter/digit token that
// looks generated rather than chosen.
func looksRandom(name string) bool {
	for _, tok := range tokenizeName(name) {
		if tok.class != classAlnum || len(tok.text) < minRandomTokenLen {
			continue
		}
		if hasMixedCase(tok.text) || letterDigitRuns(tok.text) >= minRandomTokenRuns {
			return true
		}
	}
	return false
}

// mostlyRandom reports whether more than half of names look random.
func mostlyRandom(names []string) bool {
	random := 0
	for _, n := range names {
		if looksRandom(n) {
			random++
		}
	}
	return 2*random > len(names)
}

// clusterMembers splits a signature bucket into the member sets to
// template: one per affix family sharing letters with at least
// minClusterSize members, then the remaining members together when they
// are numerous enough and mostly look like generated IDs.
func clusterMembers(members []string, minClusterSize int) [][]string {
	families := make(map[string][]string)
	var keys []string
	for _, m := range members {
		k := affixKey(m)
		if _, ok := families[k]; !ok {
			keys = append(keys, k)
		}
		families[k] = append(families[k], m)
	}
	sort.Strings(keys)
	var (
		out  [][]string
		rest []string
	)
	for _, k := range keys {
		if len(families[k]) >= minClusterSize && hasSharedLetters(k) {
			out = append(out, families[k])
		} else {
			rest = append(rest, families[k]...)
		}
	}
	if len(rest) >= minClusterSize && mostlyRandom(rest) {
		sort.Strings(rest)
		out = append(out, rest)
	}
	return out
}

type templatePart struct {
	literal string
	class   tokenClass
}

// parseTemplate splits a template into literal and placeholder parts.
// ok is false when the template holds no placeholder.
func parseTemplate(template string) (parts []templatePart, ok bool) {
	literalStart := 0
	for i := 0; i < len(template); i++ {
		if template[i] != '<' {
			continue
		}
		end := strings.IndexByte(template[i:], '>')
		if end < 0 {
			break
		}
		class, known := placeholderClasses[template[i:i+end+1]]
		if !known {
			continue
		}
		if literalStart < i {
			parts = append(parts, templatePart{literal: template[literalStart:i]})
		}
		parts = append(parts, templatePart{class: class})
		ok = true
		i += end
		literalStart = i + 1
	}
	if literalStart < len(template) {
		parts = append(parts, templatePart{literal: template[literalStart:]})
	}
	return parts, ok
}

// isPatternName reports whether name is a mined template. Literal "*"
// written by the PathsReducer is not a pattern.
func isPatternName(name string) bool {
	_, ok := parseTemplate(name)
	return ok
}

// templateScope tells which nodes a template may stand for.
type templateScope uint8

const (
	// scopeNone templates are never installed.
	scopeNone templateScope = iota
	// scopeDirectories templates only stand for directories, so that a new
	// file never matches them and the path below them is still checked.
	scopeDirectories
	scopeAll
)

// templateScopeOf returns where template may be installed. A literal
// anchor or a placeholder narrower than <alpha> and <alnum> allows any
// node. Otherwise <alnum> is limited to directories and <alpha> is
// refused, so fixed names (tmp / var / etc) never collapse into <alpha>.
func templateScopeOf(template string) templateScope {
	parts, ok := parseTemplate(template)
	if !ok {
		return scopeNone
	}
	scope := scopeNone
	for _, p := range parts {
		if p.literal != "" {
			for i := 0; i < len(p.literal); i++ {
				if !isSeparator(p.literal[i]) {
					return scopeAll
				}
			}
			continue
		}
		switch p.class {
		case classAlpha:
		case classAlnum:
			scope = scopeDirectories
		default:
			return scopeAll
		}
	}
	return scope
}

// directoryMembers returns the members that already have children. A
// directory created by the current insert is still empty, so directory-only
// templates are mostly installed by FinalizePatterns.
func directoryMembers(children map[string]*FileNode, members []string) []string {
	var out []string
	for _, name := range members {
		if c := children[name]; c != nil && len(c.Children) > 0 {
			out = append(out, name)
		}
	}
	return out
}

type compiledPattern struct {
	re *regexp.Regexp
	// specificity orders matching patterns; higher wins.
	specificity int
	// directoryOnly patterns never match the last component of a path.
	directoryOnly bool
}

func compilePattern(template string) *compiledPattern {
	parts, _ := parseTemplate(template)
	var (
		expr        strings.Builder
		specificity int
	)
	expr.WriteByte('^')
	for _, p := range parts {
		if p.literal != "" {
			expr.WriteString(regexp.QuoteMeta(p.literal))
			specificity += 10 * len(p.literal)
			continue
		}
		expr.WriteString(classes[p.class].regex)
		specificity -= classes[p.class].width
	}
	expr.WriteByte('$')
	return &compiledPattern{
		re:            regexp.MustCompile(expr.String()),
		specificity:   specificity,
		directoryOnly: templateScopeOf(template) == scopeDirectories,
	}
}

// matcher returns the compiled pattern of a pattern node, compiling it on
// first use. Rebuilt from Name, so it survives profile reloads.
func (fn *FileNode) matcher() *compiledPattern {
	if fn.pattern == nil {
		fn.pattern = compilePattern(fn.Name)
	}
	return fn.pattern
}

type signatureBucket struct {
	signature string
	members   []string
}

// groupChildrenBySignature partitions non-pattern children by structural
// signature, sorted by signature for deterministic iteration.
func groupChildrenBySignature(children map[string]*FileNode) []signatureBucket {
	byKey := make(map[string][]string)
	for name, child := range children {
		if child == nil || child.IsPattern {
			continue
		}
		sig := structureSignature(name)
		byKey[sig] = append(byKey[sig], name)
	}
	out := make([]signatureBucket, 0, len(byKey))
	for sig, names := range byKey {
		sort.Strings(names)
		out = append(out, signatureBucket{signature: sig, members: names})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].signature < out[j].signature })
	return out
}

// mergeInto folds src into dst in place: unions NodeBase observations,
// Children, MatchedRules, Open flags/mode, and keeps the more
// authoritative GenerationType. dst.Name is left to the caller.
func (dst *FileNode) mergeInto(src *FileNode) {
	if src == nil {
		return
	}
	src.EachSeen(func(id uint64, times ImageTagTimes) {
		if existing, ok := dst.GetSeenTimes(id); ok {
			firstSeen := existing.FirstSeen
			lastSeen := existing.LastSeen
			if times.FirstSeen.Before(firstSeen) {
				firstSeen = times.FirstSeen
			}
			if times.LastSeen.After(lastSeen) {
				lastSeen = times.LastSeen
			}
			dst.RecordWithTimestamps(id, firstSeen, lastSeen)
		} else {
			dst.RecordWithTimestamps(id, times.FirstSeen, times.LastSeen)
		}
	})

	dst.MatchedRules = model.AppendMatchedRule(dst.MatchedRules, src.MatchedRules)

	if dst.File == nil {
		dst.File = src.File
	}

	if src.Open != nil {
		if dst.Open == nil {
			cp := *src.Open
			dst.Open = &cp
		} else {
			dst.Open.Flags |= src.Open.Flags
			dst.Open.Mode |= src.Open.Mode
		}
	}

	if generationPriority(src.GenerationType) > generationPriority(dst.GenerationType) {
		dst.GenerationType = src.GenerationType
	}

	for name, child := range src.Children {
		if existing, ok := dst.Children[name]; ok {
			existing.mergeInto(child)
		} else {
			dst.Children[name] = child
		}
	}
}

func generationPriority(t NodeGenerationType) int {
	switch t {
	case Snapshot:
		return 4
	case Runtime:
		return 3
	case WorkloadWarmup:
		return 2
	case ProfileDrift:
		return 1
	}
	return 0
}

// collapseBucket folds members into a single pattern node named template
// and installs it in children.
func collapseBucket(children map[string]*FileNode, template string, members []string, stats *Stats) bool {
	if len(members) < 2 {
		return false
	}
	head := children[members[0]]
	if head == nil {
		return false
	}
	head.Name = template
	head.IsPattern = true
	head.pattern = nil
	for _, name := range members[1:] {
		sibling := children[name]
		if sibling == nil {
			continue
		}
		head.mergeInto(sibling)
		delete(children, name)
		if stats != nil {
			stats.FileNodes--
			stats.FileNodesMerged++
		}
	}
	if head.File != nil {
		head.File.BasenameStr = template
	}
	rewriteSubtreePaths(head, 0, template)
	delete(children, members[0])
	if existing, ok := children[template]; ok && existing != head {
		existing.mergeInto(head)
		if stats != nil {
			stats.FileNodes--
			stats.FileNodesMerged++
		}
	} else {
		children[template] = head
	}
	return true
}

// rewriteSubtreePaths replaces, in the PathnameStr of fn and its
// descendants, the path component that names the collapsed node.
// depthFromEnd is fn's distance below the collapsed node.
func rewriteSubtreePaths(fn *FileNode, depthFromEnd int, template string) {
	if fn.File != nil && fn.File.PathnameStr != "" {
		parts := strings.Split(fn.File.PathnameStr, "/")
		if idx := len(parts) - 1 - depthFromEnd; idx >= 0 {
			parts[idx] = template
			fn.File.PathnameStr = strings.Join(parts, "/")
		}
	}
	for _, child := range fn.Children {
		rewriteSubtreePaths(child, depthFromEnd+1, template)
	}
}

// mergeChildren runs one merge pass over children, collapsing every
// signature bucket with at least minClusterSize members into a pattern
// node. Returns the number of buckets collapsed.
func mergeChildren(children map[string]*FileNode, minClusterSize int, stats *Stats) int {
	if len(children) == 0 || minClusterSize < 2 {
		return 0
	}
	collapsed := 0
	for _, b := range groupChildrenBySignature(children) {
		if len(b.members) < minClusterSize {
			continue
		}
		for _, members := range clusterMembers(b.members, minClusterSize) {
			template := buildTemplate(members)
			switch templateScopeOf(template) {
			case scopeNone:
				continue
			case scopeDirectories:
				members = directoryMembers(children, members)
				if len(members) < minClusterSize || !mostlyRandom(members) {
					continue
				}
				template = buildTemplate(members)
				if templateScopeOf(template) == scopeNone {
					continue
				}
			}
			if collapseBucket(children, template, members, stats) {
				collapsed++
			}
		}
	}
	return collapsed
}

// maybeMergeChildren runs a merge pass when mining is enabled and the
// child count exceeds the configured fan-out threshold.
func maybeMergeChildren(children map[string]*FileNode, stats *Stats) int {
	cfg := pathPatternCfgFrom(stats)
	if !cfg.Enabled || cfg.MaxChildren <= 0 || len(children) <= cfg.MaxChildren {
		return 0
	}
	return mergeChildren(children, cfg.MinClusterSize, stats)
}

// insertChildAndMerge stores child under name, runs the fan-out merge pass,
// and returns the node that now owns name: child itself, or the pattern node
// it was folded into.
func insertChildAndMerge(children map[string]*FileNode, name string, child *FileNode, stats *Stats) *FileNode {
	children[name] = child
	maybeMergeChildren(children, stats)
	if owner, ok := findChildWithPatternFallback(children, name, false, stats); ok {
		return owner
	}
	children[name] = child
	return child
}

// findChildWithPatternFallback returns the exact-name child if present,
// otherwise the most specific sibling pattern node matching name.
// lastComponent excludes directory-only patterns, so a new file never
// matches them. A disabled config skips the pattern scan entirely.
func findChildWithPatternFallback(children map[string]*FileNode, name string, lastComponent bool, stats *Stats) (*FileNode, bool) {
	if c, ok := children[name]; ok {
		return c, true
	}
	if !pathPatternCfgFrom(stats).Enabled {
		return nil, false
	}
	var best *FileNode
	for _, c := range children {
		if c == nil || !c.IsPattern {
			continue
		}
		m := c.matcher()
		if (lastComponent && m.directoryOnly) || !m.re.MatchString(name) {
			continue
		}
		if best == nil {
			best = c
			continue
		}
		bm := best.matcher()
		if m.specificity > bm.specificity || (m.specificity == bm.specificity && c.Name < best.Name) {
			best = c
		}
	}
	return best, best != nil
}

// rulePathFromProfilePath converts a profile path to a SECL path value,
// turning typed placeholders into "*" globs.
func rulePathFromProfilePath(path string) string {
	parts, ok := parseTemplate(path)
	if !ok {
		return pathutils.CheckForPatterns(path)
	}
	var glob strings.Builder
	for _, p := range parts {
		if p.literal != "" {
			glob.WriteString(p.literal)
		} else {
			glob.WriteByte('*')
		}
	}
	out := pathutils.CheckForPatterns(glob.String())
	if !strings.HasPrefix(out, "~") {
		out = "~" + out
	}
	return out
}

func pathPatternCfgFrom(stats *Stats) PathPatternConfig {
	if stats == nil {
		return PathPatternConfig{}
	}
	return stats.patternCfg
}
