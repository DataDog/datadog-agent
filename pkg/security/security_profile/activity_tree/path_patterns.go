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
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils/pathutils"
)

// PathPatternConfig controls path-pattern merging of sibling FileNodes.
// Merging is opt-in per ActivityTree via Stats.SetPathPatternConfig; v2
// security profiles enable it, v1 profiles and activity dumps leave it
// off.
type PathPatternConfig struct {
	Enabled bool
	// MaxChildren is the child count above which inserts run merge rounds
	// with MinGroupSizeOnInsert, the next one once the count has doubled.
	MaxChildren          int
	MinGroupSizeOnInsert int
	// MinGroupSize is the threshold of the FinalizePatterns pass run when
	// a profile is saved.
	MinGroupSize int
	// MinDateGroupSize lowers the group size needed when the template
	// replaces a date: a dated name is already strong evidence of
	// rotation. Ignored when not below the pass threshold.
	MinDateGroupSize int
}

// DefaultPathPatternConfig returns the enabled configuration used by v2
// security profiles.
func DefaultPathPatternConfig() PathPatternConfig {
	return PathPatternConfig{
		Enabled:              true,
		MaxChildren:          15,
		MinGroupSizeOnInsert: 5,
		MinGroupSize:         3,
		MinDateGroupSize:     2,
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
	classDate
	// classContainerID and classAny are only written by the PathsReducer;
	// the tokenizer never produces them.
	classContainerID
	classAny
)

// minHexLen is the shortest piece classified as a hex identifier, so
// short words made of a-f letters ("cafe", "bad") stay alpha.
const minHexLen = 8

type classInfo struct {
	code        string
	placeholder string
	// accepts reports whether a whole text belongs to the class. The
	// tokenizer and the matcher both use it, so they can't disagree.
	accepts func(string) bool
	// width ranks how much a placeholder accepts; lower is narrower.
	width int
}

var classes = [...]classInfo{
	classNum:         {code: "N", placeholder: "<num>", accepts: allDigits, width: 1},
	classUUID:        {code: "U", placeholder: "<uuid>", accepts: isUUID, width: 1},
	classHex:         {code: "H", placeholder: "<hex>", accepts: isHexID, width: 2},
	classAlpha:       {code: "A", placeholder: "<alpha>", accepts: allLetters, width: 3},
	classAlnum:       {code: "M", placeholder: "<alnum>", accepts: isMixedAlnum, width: 4},
	classDate:        {code: "D", placeholder: "<date>", accepts: isDate, width: 1},
	classContainerID: {code: "C", placeholder: "<container_id>", accepts: isContainerID, width: 1},
	// <any> must always sit between literals, or it would match every sibling.
	classAny: {code: "X", placeholder: "<any>", accepts: func(s string) bool { return !strings.Contains(s, "/") }, width: 5},
}

// pieceClasses are tried in order on a piece between separators.
var pieceClasses = [...]tokenClass{classNum, classHex, classAlpha, classAlnum}

var containerIDPattern = regexp.MustCompile(`^(?:` + containerutils.ContainerIDPatternStr + `)$`)

var placeholderClasses = map[string]tokenClass{
	classes[classNum].placeholder:         classNum,
	classes[classUUID].placeholder:        classUUID,
	classes[classHex].placeholder:         classHex,
	classes[classAlpha].placeholder:       classAlpha,
	classes[classAlnum].placeholder:       classAlnum,
	classes[classDate].placeholder:        classDate,
	classes[classContainerID].placeholder: classContainerID,
	classes[classAny].placeholder:         classAny,
}

type nameToken struct {
	text  string
	class tokenClass
}

// tokenizeName splits name into separator, UUID, date and piece tokens.
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
		if n := dateLenAt(name, i); n > 0 {
			out = append(out, nameToken{text: name[i : i+n], class: classDate})
			i += n
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
	for _, class := range pieceClasses {
		if classes[class].accepts(piece) {
			return append(out, nameToken{text: piece, class: class})
		}
	}
	return append(out, nameToken{text: piece, class: classLiteral})
}

func isHexID(s string) bool {
	if len(s) < minHexLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHexChar(s[i]) {
			return false
		}
	}
	return true
}

func allLetters(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isAlpha(s[i]) {
			return false
		}
	}
	return s != ""
}

// isMixedAlnum reports whether s holds only letters and digits, with at
// least one of each.
func isMixedAlnum(s string) bool {
	hasAlpha, hasDigit := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case isDigit(c):
			hasDigit = true
		case isAlpha(c):
			hasAlpha = true
		default:
			return false
		}
	}
	return hasAlpha && hasDigit
}

func isContainerID(s string) bool {
	return containerIDPattern.MatchString(s)
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

const uuidLen = 36

// isUUIDAt reports whether a UUID starts at name[i] and ends at a separator
// or the end of name.
func isUUIDAt(name string, i int) bool {
	end := i + uuidLen
	return end <= len(name) && isTokenEnd(name, end) && isUUID(name[i:end])
}

// isUUID reports whether s is a canonical 8-4-4-4-12 UUID.
func isUUID(s string) bool {
	if len(s) != uuidLen {
		return false
	}
	for k := 0; k < uuidLen; k++ {
		c := s[k]
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

// dateLenAt returns the length of the date starting at name[i], with an
// optional time of day, or 0. Accepted forms: YYYY-MM-DD (or with _ or .),
// YYYYMMDD, then optionally THHMMSS, -HH-MM-SS, _HH:MM:SS (Z allowed).
// The year must be 19xx or 20xx and the month and day valid, so plain
// numbers stay <num>. The date must end at a separator or the end of name.
func dateLenAt(name string, i int) int {
	n := calendarDateLen(name[i:])
	if n == 0 {
		return 0
	}
	if t := timeOfDayLen(name[i+n:]); t > 0 && isTokenEnd(name, i+n+t) {
		return n + t
	}
	if isTokenEnd(name, i+n) {
		return n
	}
	return 0
}

// isDate reports whether s is exactly a date in one of the dateLenAt forms.
func isDate(s string) bool {
	n := calendarDateLen(s)
	return n > 0 && (n == len(s) || n+timeOfDayLen(s[n:]) == len(s))
}

func calendarDateLen(s string) int {
	if len(s) >= 10 && isSeparator(s[4]) && s[7] == s[4] &&
		isYear(s[:4]) && isTwoDigitsIn(s[5:7], 1, 12) && isTwoDigitsIn(s[8:10], 1, 31) {
		return 10
	}
	if len(s) >= 8 && isYear(s[:4]) && isTwoDigitsIn(s[4:6], 1, 12) && isTwoDigitsIn(s[6:8], 1, 31) {
		return 8
	}
	return 0
}

func timeOfDayLen(s string) int {
	if len(s) < 7 || (s[0] != 'T' && s[0] != '-' && s[0] != '_') || !isTwoDigitsIn(s[1:3], 0, 23) {
		return 0
	}
	j := 3
	var sep byte
	if isTimeSeparator(s[j]) {
		sep = s[j]
		j++
	}
	if len(s) < j+2 || !isTwoDigitsIn(s[j:j+2], 0, 59) {
		return 0
	}
	j += 2
	if sep != 0 {
		if len(s) <= j || s[j] != sep {
			return 0
		}
		j++
	}
	if len(s) < j+2 || !isTwoDigitsIn(s[j:j+2], 0, 59) {
		return 0
	}
	j += 2
	if j < len(s) && s[j] == 'Z' {
		j++
	}
	return j
}

func isYear(s string) bool {
	return (s[:2] == "19" || s[:2] == "20") && isDigit(s[2]) && isDigit(s[3])
}

func isTwoDigitsIn(s string, lo, hi int) bool {
	if !isDigit(s[0]) || !isDigit(s[1]) {
		return false
	}
	v := int(s[0]-'0')*10 + int(s[1]-'0')
	return lo <= v && v <= hi
}

func isTimeSeparator(c byte) bool {
	return isSeparator(c) || c == ':'
}

func isTokenEnd(name string, j int) bool {
	return j == len(name) || isSeparator(name[j])
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isAlpha(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isSeparator(c byte) bool {
	return c == '-' || c == '.' || c == '_'
}

// structureSignature returns a canonical skeleton of name: separators
// kept literally, one class code per token, and unusual pieces quoted
// with NUL (which cannot appear in a file name) so they never collide.
// Examples: "sess-42" -> "A-N", "sess42" -> "M", "2024-01-15.log" -> "D.A".
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

// tokenizedName holds a sibling name and its tokens for one merge pass.
type tokenizedName struct {
	name   string
	tokens []nameToken
}

func tokenizeNames(names []string) []tokenizedName {
	out := make([]tokenizedName, len(names))
	for i, name := range names {
		out[i] = tokenizedName{name: name, tokens: tokenizeName(name)}
	}
	return out
}

func namesOf(members []tokenizedName) []string {
	out := make([]string, len(members))
	for i, member := range members {
		out[i] = member.name
	}
	return out
}

// buildTemplate returns the merged name for a cluster of same-signature
// siblings, keeping tokens on which all siblings agree and replacing the
// others with the placeholder of their class. Mixed letter/digit tokens
// keep the letter prefix and suffix shared by all siblings.
// Examples: [sess-1, sess-22] -> "sess-<num>", [sess1, sess22] -> "sess<num>".
func buildTemplate(members []tokenizedName) string {
	if len(members) == 0 {
		return ""
	}
	if len(members) == 1 {
		return members[0].name
	}
	ref := members[0].tokens
	var out strings.Builder
	texts := make([]string, len(members))
	for k, tok := range ref {
		allSame := true
		for i, member := range members {
			if k >= len(member.tokens) {
				allSame = false
				texts[i] = ""
				continue
			}
			texts[i] = member.tokens[k].text
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
// letter/digit token of a name, so that families such as sess1, sess2 are
// split from unrelated random IDs sharing the same signature.
func affixKey(tokens []nameToken) string {
	var key strings.Builder
	for _, tok := range tokens {
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

// hasLetterAffix reports whether an affixKey carries a letter prefix or suffix.
func hasLetterAffix(key string) bool {
	return strings.Trim(key, "\x00") != ""
}

// digitsOnlyKey marks each mixed token whose part between letters is digits only.
func digitsOnlyKey(tokens []nameToken) string {
	var key []byte
	for _, tok := range tokens {
		if tok.class != classAlnum {
			continue
		}
		middle := tok.text[len(leadingLetters(tok.text)) : len(tok.text)-len(trailingLetters(tok.text))]
		if allDigits(middle) {
			key = append(key, 'n')
		} else {
			key = append(key, 'a')
		}
	}
	return string(key)
}

// splitDigitsOnly keeps sess1 (sess<num>) and sess4a2 (sess<alnum>) apart.
func splitDigitsOnly(family []tokenizedName, minGroupSize int) [][]tokenizedName {
	parts := make(map[string][]tokenizedName)
	var keys []string
	for _, member := range family {
		key := digitsOnlyKey(member.tokens)
		if _, ok := parts[key]; !ok {
			keys = append(keys, key)
		}
		parts[key] = append(parts[key], member)
	}
	sort.Strings(keys)
	var out [][]tokenizedName
	for _, key := range keys {
		if len(parts[key]) >= minGroupSize {
			out = append(out, parts[key])
		}
	}
	return out
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

// looksRandom reports whether a name holds a mixed letter/digit token that
// looks generated rather than chosen.
func looksRandom(tokens []nameToken) bool {
	for _, tok := range tokens {
		if tok.class != classAlnum || len(tok.text) < minRandomTokenLen {
			continue
		}
		if hasMixedCase(tok.text) || letterDigitRuns(tok.text) >= minRandomTokenRuns {
			return true
		}
	}
	return false
}

// mostlyRandom reports whether more than half of members look random.
func mostlyRandom(members []tokenizedName) bool {
	random := 0
	for _, member := range members {
		if looksRandom(member.tokens) {
			random++
		}
	}
	return 2*random > len(members)
}

// clusterMembers splits a signature bucket into the member sets to
// template: the parts of every affix family sharing letters with at least
// minGroupSize members, then the remaining members together when they
// are numerous enough and mostly look like generated IDs.
func clusterMembers(members []tokenizedName, minGroupSize int) [][]tokenizedName {
	families := make(map[string][]tokenizedName)
	var keys []string
	for _, member := range members {
		key := affixKey(member.tokens)
		if _, ok := families[key]; !ok {
			keys = append(keys, key)
		}
		families[key] = append(families[key], member)
	}
	sort.Strings(keys)
	var (
		out  [][]tokenizedName
		rest []tokenizedName
	)
	for _, key := range keys {
		// An empty key means no mixed token (sess-1, sess-2): nothing to split.
		if len(families[key]) >= minGroupSize && (key == "" || hasLetterAffix(key)) {
			out = append(out, splitDigitsOnly(families[key], minGroupSize)...)
		} else {
			rest = append(rest, families[key]...)
		}
	}
	if len(rest) >= minGroupSize && mostlyRandom(rest) {
		sort.Slice(rest, func(i, j int) bool { return rest[i].name < rest[j].name })
		out = append(out, rest)
	}
	return out
}

type templatePart struct {
	literal string
	class   tokenClass
}

// parseTemplate splits a template into literal and placeholder parts.
// hasPlaceholder is false when the template holds no placeholder.
func parseTemplate(template string) (parts []templatePart, hasPlaceholder bool) {
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
		hasPlaceholder = true
		i += end
		literalStart = i + 1
	}
	if literalStart < len(template) {
		parts = append(parts, templatePart{literal: template[literalStart:]})
	}
	return parts, hasPlaceholder
}

// isPatternName reports whether name reads as a template. Real file names
// can too: FileNode.IsPattern tells whether a node is a pattern.
func isPatternName(name string) bool {
	if strings.IndexByte(name, '<') < 0 {
		return false
	}
	_, hasPlaceholder := parseTemplate(name)
	return hasPlaceholder
}

// reducedComponentIsPattern reports whether the PathsReducer wrote a
// placeholder in name, rawName being the component before reduction.
func reducedComponentIsPattern(name, rawName string) bool {
	return name != rawName && !isPatternName(rawName) && isPatternName(name)
}

// isMergeCandidate excludes patterns, names that read as a template, and
// invalid UTF-8, which the profile encoding rewrites.
func isMergeCandidate(name string, child *FileNode) bool {
	return child != nil && !child.IsPattern && utf8.ValidString(name) && !isPatternName(name)
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
	parts, hasPlaceholder := parseTemplate(template)
	if !hasPlaceholder {
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
func directoryMembers(children map[string]*FileNode, members []tokenizedName) []tokenizedName {
	var out []tokenizedName
	for _, member := range members {
		if c := children[member.name]; c != nil && len(c.Children) > 0 {
			out = append(out, member)
		}
	}
	return out
}

type compiledPattern struct {
	parts []templatePart
	// specificity orders matching patterns; higher wins.
	specificity int
	// directoryOnly patterns only match directories.
	directoryOnly bool
}

func compilePattern(template string) *compiledPattern {
	parts, _ := parseTemplate(template)
	specificity := 0
	for _, p := range parts {
		if p.literal != "" {
			specificity += 10 * len(p.literal)
		} else {
			specificity -= classes[p.class].width
		}
	}
	return &compiledPattern{
		parts:         parts,
		specificity:   specificity,
		directoryOnly: templateScopeOf(template) == scopeDirectories,
	}
}

func (p *compiledPattern) matches(name string) bool {
	return matchParts(p.parts, name)
}

// matchParts reports whether name splits into one text per part: the
// literal itself, or a text the placeholder's class accepts. Longer texts
// are tried first.
func matchParts(parts []templatePart, name string) bool {
	if len(parts) == 0 {
		return name == ""
	}
	if lit := parts[0].literal; lit != "" {
		return strings.HasPrefix(name, lit) && matchParts(parts[1:], name[len(lit):])
	}
	accepts, rest := classes[parts[0].class].accepts, parts[1:]
	if len(rest) == 0 {
		return accepts(name)
	}
	for end := len(name); end >= 0; end-- {
		if lit := rest[0].literal; lit != "" && !strings.HasPrefix(name[end:], lit) {
			continue
		}
		if accepts(name[:end]) && matchParts(rest, name[end:]) {
			return true
		}
	}
	return false
}

// matcher returns the compiled pattern of a pattern node, compiling it on
// first use. Rebuilt from Name, so it survives profile reloads.
func (fn *FileNode) matcher() *compiledPattern {
	if fn.pattern == nil {
		fn.pattern = compilePattern(fn.Name)
	}
	return fn.pattern
}

// signatureOf returns the structure signature of name, cached on fn when
// name is fn's own name.
func (fn *FileNode) signatureOf(name string) string {
	if name != fn.Name {
		return structureSignature(name)
	}
	if fn.signature == "" {
		fn.signature = structureSignature(name)
	}
	return fn.signature
}

type signatureBucket struct {
	signature string
	members   []string
}

// groupChildrenBySignature partitions the merge candidates of children by
// structural signature, sorted by signature for deterministic iteration.
func groupChildrenBySignature(children map[string]*FileNode) []signatureBucket {
	byKey := make(map[string][]string)
	for name, child := range children {
		if !isMergeCandidate(name, child) {
			continue
		}
		sig := child.signatureOf(name)
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

// mergeInto folds src into fn in place: unions NodeBase observations,
// Children, MatchedRules, Open flags/mode, and keeps the more
// authoritative GenerationType. fn.Name is left to the caller. Every node
// dropped on the way is recorded in stats as moved to its new owner.
func (fn *FileNode) mergeInto(src *FileNode, stats *Stats) {
	if src == nil {
		return
	}
	stats.recordMovedNode(&src.NodeBase, &fn.NodeBase)
	fn.mergeSeen(&src.NodeBase)

	fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, src.MatchedRules)

	if fn.File == nil {
		fn.File = src.File
	}

	if src.Open != nil {
		if fn.Open == nil {
			cp := *src.Open
			fn.Open = &cp
		} else {
			fn.Open.Flags |= src.Open.Flags
			fn.Open.Mode |= src.Open.Mode
		}
	}

	if generationPriority(src.GenerationType) > generationPriority(fn.GenerationType) {
		fn.GenerationType = src.GenerationType
	}

	if len(src.Children) > 0 && fn.Children == nil {
		fn.Children = make(map[string]*FileNode, len(src.Children))
	}
	for name, child := range src.Children {
		if existing, ok := fn.Children[name]; ok {
			existing.mergeInto(child, stats)
			if stats != nil {
				stats.FileNodes--
				stats.FileNodesMerged++
			}
		} else {
			fn.Children[name] = child
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
	sizeBefore := stringMapBytes(children)
	for _, name := range members {
		if c := children[name]; c != nil {
			sizeBefore += fileSubtreeSizeBytes(c)
		}
	}
	existing := children[template]
	if existing != nil {
		sizeBefore += fileSubtreeSizeBytes(existing)
	}

	head.Name = template
	head.IsPattern = true
	head.pattern = nil
	head.signature = ""
	for _, name := range members[1:] {
		sibling := children[name]
		if sibling == nil {
			continue
		}
		head.mergeInto(sibling, stats)
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
	owner := head
	if existing != nil && existing != head {
		existing.mergeInto(head, stats)
		// existing may be a literal file whose name equals the template.
		existing.IsPattern = true
		owner = existing
		if stats != nil {
			stats.FileNodes--
			stats.FileNodesMerged++
		}
	}
	children[template] = owner
	if stats != nil {
		stats.SizeBytes += stringMapBytes(children) + fileSubtreeSizeBytes(owner) - sizeBefore
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
// signature bucket with at least minGroupSize members into a pattern
// node. Templates that replace a date only need MinDateGroupSize
// members. Returns the number of buckets collapsed.
func mergeChildren(children map[string]*FileNode, minGroupSize int, stats *Stats) int {
	if len(children) == 0 || minGroupSize < 2 {
		return 0
	}
	return mergeBuckets(children, groupChildrenBySignature(children), minGroupSize, stats)
}

func mergeBuckets(children map[string]*FileNode, buckets []signatureBucket, minGroupSize int, stats *Stats) int {
	dateGroupSize := minGroupSize
	if n := pathPatternCfgFrom(stats).MinDateGroupSize; n >= 2 && n < minGroupSize {
		dateGroupSize = n
	}
	datePlaceholder := classes[classDate].placeholder
	collapsed := 0
	for _, b := range buckets {
		threshold := minGroupSize
		if strings.Contains(b.signature, classes[classDate].code) {
			threshold = dateGroupSize
		}
		if len(b.members) < threshold || !canYieldTemplate(b.signature) {
			continue
		}
		enough := func(members []tokenizedName, template string) bool {
			return len(members) >= minGroupSize || (len(members) >= threshold && strings.Contains(template, datePlaceholder))
		}
		for _, members := range clusterMembers(tokenizeNames(b.members), threshold) {
			template := buildTemplate(members)
			if !enough(members, template) {
				continue
			}
			switch templateScopeOf(template) {
			case scopeNone:
				continue
			case scopeDirectories:
				members = directoryMembers(children, members)
				if len(members) < minGroupSize || !mostlyRandom(members) {
					continue
				}
				template = buildTemplate(members)
				if templateScopeOf(template) == scopeNone {
					continue
				}
			}
			// a template must match every name it absorbs
			members = matchingMembers(template, members)
			if !enough(members, template) {
				continue
			}
			if collapseBucket(children, template, namesOf(members), stats) {
				collapsed++
			}
		}
	}
	return collapsed
}

// canYieldTemplate is false for a lone alpha token: a bare <alpha> is always refused.
func canYieldTemplate(signature string) bool {
	return signature != classes[classAlpha].code
}

func matchingMembers(template string, members []tokenizedName) []tokenizedName {
	pattern := compilePattern(template)
	var out []tokenizedName
	for _, member := range members {
		if pattern.matches(member.name) {
			out = append(out, member)
		}
	}
	return out
}

// foldCoveredPatterns folds every pattern into a sibling pattern matching all
// its names. Only childless patterns fold: merging subtrees would widen them.
func foldCoveredPatterns(children map[string]*FileNode, stats *Stats) int {
	var patterns []string
	for name, child := range children {
		if child != nil && child.IsPattern && len(child.Children) == 0 {
			patterns = append(patterns, name)
		}
	}
	if len(patterns) < 2 {
		return 0
	}
	sort.Strings(patterns)
	folded := 0
	for _, name := range patterns {
		narrow := children[name]
		var broad *FileNode
		for _, other := range patterns {
			b := children[other]
			if b == nil || b == narrow || !patternCovers(b, narrow) {
				continue
			}
			if broad == nil || b.matcher().specificity > broad.matcher().specificity {
				broad = b
			}
		}
		if broad == nil {
			continue
		}
		sizeBefore := stringMapBytes(children) + fileSubtreeSizeBytes(broad) + fileSubtreeSizeBytes(narrow)
		broad.mergeInto(narrow, stats)
		delete(children, name)
		folded++
		if stats != nil {
			stats.FileNodes--
			stats.FileNodesMerged++
			stats.SizeBytes += stringMapBytes(children) + fileSubtreeSizeBytes(broad) - sizeBefore
		}
	}
	return folded
}

// patternCovers reports whether broad matches every name narrow matches. It
// compares the pieces between separators, so it may miss a cover, never invent one.
func patternCovers(broad, narrow *FileNode) bool {
	b, n := broad.matcher(), narrow.matcher()
	if b.specificity >= n.specificity || (b.directoryOnly && !n.directoryOnly) {
		return false
	}
	bp, np := templatePieces(broad.Name), templatePieces(narrow.Name)
	if len(bp) != len(np) {
		return false
	}
	for i := range bp {
		if !pieceCovers(bp[i], np[i]) {
			return false
		}
	}
	return true
}

// templatePieces splits a template on the separators of its literal parts,
// keeping the separators.
func templatePieces(template string) []string {
	parts, _ := parseTemplate(template)
	var (
		out []string
		cur strings.Builder
	)
	for _, p := range parts {
		if p.literal == "" {
			cur.WriteString(classes[p.class].placeholder)
			continue
		}
		for i := 0; i < len(p.literal); i++ {
			if isSeparator(p.literal[i]) {
				out = append(out, cur.String(), p.literal[i:i+1])
				cur.Reset()
			} else {
				cur.WriteByte(p.literal[i])
			}
		}
	}
	return append(out, cur.String())
}

func pieceCovers(broad, narrow string) bool {
	switch {
	case broad == narrow || broad == classes[classAny].placeholder:
		return true
	case !isPatternName(narrow):
		return compilePattern(broad).matches(narrow)
	case broad == classes[classAlnum].placeholder:
		return alwaysMixed(narrow)
	}
	return false
}

// alwaysMixed reports whether every name matching piece mixes letters and
// digits, as sess<num> or pod<alnum> do.
func alwaysMixed(piece string) bool {
	parts, _ := parseTemplate(piece)
	letters, placeholders := 0, 0
	class := classLiteral
	for _, p := range parts {
		if p.literal == "" {
			placeholders++
			class = p.class
			continue
		}
		for i := 0; i < len(p.literal); i++ {
			if !isAlpha(p.literal[i]) {
				return false
			}
		}
		letters += len(p.literal)
	}
	return placeholders == 1 && (class == classAlnum || (class == classNum && letters > 0))
}

// childIndex holds, for one children map, its pattern nodes, so that a
// lookup miss only checks those, and the size of its next merge round.
type childIndex struct {
	patterns []*FileNode
	// indexedLen is len(children) when patterns was built. Code that removes
	// or swaps children outside the insert path must call reset.
	indexedLen int
	nextMerge  int
}

// indexOf returns *index, allocating it on first use.
func indexOf(index **childIndex) *childIndex {
	if *index == nil {
		*index = &childIndex{}
	}
	return *index
}

func (idx *childIndex) reset() {
	if idx != nil {
		idx.patterns, idx.indexedLen = nil, -1
	}
}

func (idx *childIndex) patternsOf(children map[string]*FileNode) []*FileNode {
	if idx.indexedLen != len(children) {
		idx.patterns = idx.patterns[:0]
		for _, child := range children {
			if child != nil && child.IsPattern {
				idx.patterns = append(idx.patterns, child)
			}
		}
		idx.indexedLen = len(children)
	}
	return idx.patterns
}

// added keeps idx valid after the insert path stored child in children. An
// index that was already stale is reset, so the new count can't revive it.
func (idx *childIndex) added(children map[string]*FileNode, child *FileNode) {
	if idx.indexedLen != len(children)-1 {
		idx.reset()
		return
	}
	idx.indexedLen++
	if child.IsPattern {
		idx.patterns = append(idx.patterns, child)
	}
}

// mergeRound runs a merge pass over children once they outgrow MaxChildren,
// then waits for them to double, so siblings that never merge cost O(1)
// amortized per insert.
func mergeRound(children map[string]*FileNode, idx *childIndex, stats *Stats) {
	cfg := pathPatternCfgFrom(stats)
	if cfg.MaxChildren <= 0 || cfg.MinGroupSizeOnInsert < 2 || len(children) <= cfg.MaxChildren || len(children) < idx.nextMerge {
		return
	}
	if mergeChildren(children, cfg.MinGroupSizeOnInsert, stats)+foldCoveredPatterns(children, stats) > 0 {
		idx.reset()
	}
	idx.nextMerge = 2 * len(children)
}

// insertChildAndMerge stores child under name, charges stats for child and
// its map entry, runs a merge round when due, and returns the node that now
// owns name: child itself, or the pattern node it was folded into.
func insertChildAndMerge(children map[string]*FileNode, index **childIndex, name string, child *FileNode, stats *Stats) *FileNode {
	children[name] = child
	if stats != nil {
		stats.SizeBytes += child.size() + stringMapEntryBytes(children, name)
	}
	if !pathPatternCfgFrom(stats).Enabled {
		return child
	}
	idx := indexOf(index)
	idx.added(children, child)
	mergeRound(children, idx, stats)
	if owner, ok := findChildWithPatternFallback(children, index, name, false, stats); ok {
		return owner
	}
	children[name] = child
	return child
}

// findChildWithPatternFallback returns the exact-name child if present,
// otherwise the most specific sibling pattern node matching name.
// notDirectory, set when name ends the path and isn't a directory, excludes
// directory-only patterns, so a file never matches them. A disabled config
// skips the pattern lookup entirely.
func findChildWithPatternFallback(children map[string]*FileNode, index **childIndex, name string, notDirectory bool, stats *Stats) (*FileNode, bool) {
	if c, ok := children[name]; ok {
		return c, true
	}
	if len(children) == 0 || !pathPatternCfgFrom(stats).Enabled {
		return nil, false
	}
	var best *FileNode
	for _, child := range indexOf(index).patternsOf(children) {
		childCompiledPattern := child.matcher()
		if (notDirectory && childCompiledPattern.directoryOnly) || !childCompiledPattern.matches(name) {
			continue
		}
		if best == nil {
			best = child
			continue
		}
		bestCompiledPattern := best.matcher()
		if childCompiledPattern.specificity > bestCompiledPattern.specificity ||
			(childCompiledPattern.specificity == bestCompiledPattern.specificity && child.Name < best.Name) {
			best = child
		}
	}
	return best, best != nil
}

// withPatternComponent replaces, in path, the component name that starts
// its suffix rest with owner, the name of the node that took it.
func withPatternComponent(path, rest, name, owner string) string {
	if name == owner || !strings.HasSuffix(path, rest) {
		return path
	}
	start := len(path) - len(rest)
	if strings.HasPrefix(rest, "/") {
		start++
	}
	if !strings.HasPrefix(path[start:], name) {
		return path
	}
	return path[:start] + owner + path[start+len(name):]
}

// rulePathFromProfilePath converts a profile path to a SECL path value, globbing
// the components flagged in isPattern (the flags of its last components).
func rulePathFromProfilePath(path string, isPattern []bool) string {
	components := strings.Split(path, "/")
	offset := len(components) - len(isPattern)
	globbed := false
	for i, pattern := range isPattern {
		if !pattern || offset+i < 0 {
			continue
		}
		components[offset+i] = templateGlob(components[offset+i])
		globbed = true
	}
	out := pathutils.CheckForPatterns(strings.Join(components, "/"))
	if globbed && !strings.HasPrefix(out, "~") {
		out = "~" + out
	}
	return out
}

// templateGlob turns the placeholders of template into "*" globs.
func templateGlob(template string) string {
	parts, _ := parseTemplate(template)
	var glob strings.Builder
	for _, p := range parts {
		if p.literal != "" {
			glob.WriteString(p.literal)
		} else {
			glob.WriteByte('*')
		}
	}
	return glob.String()
}

func pathPatternCfgFrom(stats *Stats) PathPatternConfig {
	if stats == nil {
		return PathPatternConfig{}
	}
	return stats.patternCfg
}
