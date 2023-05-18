package api

import (
	"bytes"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/davecgh/go-spew/spew"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
	"strings"
	"text/template"
)

type spanChangeAttrs struct {
	AttributeMap map[string]string `yaml:"attribute_map"`
	ApplyToSpans []string          `yaml:"apply_to_spans"`
}

type spanChanges struct {
	RenameAttributes      *spanChangeAttrs `yaml:"rename_attributes"`
	ChangeAttributeValues *spanChangeAttrs `yaml:"change_attribute_values"`
}

type spanMigration struct {
	Changes []*spanChanges `yaml:"changes"`
}

type migration struct {
	Spans *spanMigration `yaml:"spans"`
}

type spanMigrationConfig struct {
	Migrations []*migration `yaml:"migrations"`
}

var (
	spanMigrationCfg *spanMigrationConfig
)

func init() {
	cfg := spanMigrationConfig{}
	yamlFile, err := os.ReadFile("./migrations-test.yml")
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		panic(err)
	}
	spanMigrationCfg = &cfg
}

func migrateSpans(tp *pb.TracerPayload) {
	// fmt.Println("------- BEFORE -------")
	// spew.Dump(tp)
	// fmt.Println("----------------------")

	for _, m := range spanMigrationCfg.Migrations {
		for _, cg := range m.Spans.Changes {
			switch {
			case cg.RenameAttributes != nil:
				applyRenaming(tp, cg.RenameAttributes)
			case cg.ChangeAttributeValues != nil:
				applyValueChanges(tp, cg.ChangeAttributeValues)
			}
		}
	}

	// fmt.Println("------- AFTER -------")
	// spew.Dump(tp)
	// fmt.Println("----------------------")
}

func applyRenaming(tp *pb.TracerPayload, attrs *spanChangeAttrs) {
	for _, ch := range tp.GetChunks() {
		for _, s := range ch.GetSpans() {
			if !appliesToSpan(s, attrs.ApplyToSpans) {
				continue
			}
			log.Infof("applies to span: %s | %s", spew.Sdump(s), spew.Sdump(attrs))
			for from, to := range attrs.AttributeMap {
				remapSpanMeta(s, from, to)
				remapSpanMetrics(s, from, to)
			}
		}
	}
}

func remapSpanMeta(s *pb.Span, from, to string) {
	val, ok := s.Meta[from]
	if !ok {
		return
	}
	s.Meta[to] = val
	delete(s.Meta, from)
}

func remapSpanMetrics(s *pb.Span, from, to string) {
	val, ok := s.Metrics[from]
	if !ok {
		return
	}
	s.Metrics[to] = val
	delete(s.Metrics, from)
}

func applyValueChanges(tp *pb.TracerPayload, attrs *spanChangeAttrs) {
	for _, ch := range tp.GetChunks() {
		for _, s := range ch.GetSpans() {
			if !appliesToSpan(s, attrs.ApplyToSpans) {
				continue
			}
			for tag, tplVal := range attrs.AttributeMap {
				newVal, ok := resolveTemplatedValue(s, tplVal)
				if !ok {
					return
				}
				if tag == "name" {
					s.Name = newVal
					continue
				}
				if tag == "service.name" {
					s.Service = newVal
				}
				applyValChangeSpanMeta(s, tag, newVal)
				applyValChangeSpanMetrics(s, tag, newVal)
			}
		}
	}
}

func applyValChangeSpanMeta(s *pb.Span, tag, val string) {
	s.Meta[tag] = val
}

func applyValChangeSpanMetrics(s *pb.Span, tag, val string) {
	n, err := strconv.ParseFloat(val, 64)
	if err == nil {
		s.Metrics[tag] = n
	}
}

func resolveTemplatedValue(s *pb.Span, tplStr string) (string, bool) {
	tplCtx := templateContext(s)
	tpl, err := template.New("expression.tpl").Funcs(template.FuncMap{
		"hasTag": func(tag string) bool {
			_, ok := tplCtx.Tags[tag]
			return ok
		},
		"eqTag": func(tag string, cmpVal string) bool {
			val, ok := tplCtx.Tags[tag]
			if !ok {
				return false
			}
			return val == cmpVal
		},
		"eqTagAny": func(tag string, cmpVals ...string) bool {
			val, ok := tplCtx.Tags[tag]
			if !ok {
				return false
			}
			for _, cmpVal := range cmpVals {
				if val == cmpVal {
					return true
				}
			}
			return false
		},
		"asMetric": func(val any) int64 {
			n, _ := strconv.ParseInt(val.(string), 10, 64)
			return n
		},
		"getTag": func(tag string) any {
			return tplCtx.Tags[tag]
		},
		"contains": func(val any, substr string) bool {
			strVal := val.(string)
			return strings.Contains(strVal, substr)
		},
		"split": func(val any, sep string) []string {
			strVal := val.(string)
			return strings.Split(strVal, sep)
		},
	}).Parse(tplStr)
	if err != nil {
		log.Errorf("failed to parse templated string: %s", err.Error())
		return "", false
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, tplCtx); err != nil {
		log.Errorf("failed to execute template: %s", err.Error())
		return "", false
	}
	return out.String(), true
}

type tplContext struct {
	Name string
	Tags map[string]any
}

func templateContext(s *pb.Span) *tplContext {
	tplCtx := &tplContext{}
	allTags := map[string]any{}
	for tag, val := range s.Meta {
		allTags[tag] = val
	}
	for tag, val := range s.Metrics {
		allTags[tag] = val
	}
	tplCtx.Tags = allTags
	tplCtx.Name = s.Name
	return tplCtx
}

func appliesToSpan(s *pb.Span, filters []string) (b bool) {
	if len(filters) == 0 {
		return true
	}
	joinedFilter := ""
	if len(filters) == 1 {
		joinedFilter = fmt.Sprintf("(%s)", filters[0])
	} else {
		joinedFilter = "and "
		for i := 0; i < len(filters); i++ {
			f := filters[i]
			joinedFilter = joinedFilter + fmt.Sprintf("(%s)", f)
			if i < len(filters)-1 {
				joinedFilter = joinedFilter + " "
			}
		}
	}

	tf := fmt.Sprintf("{{if %s}}true{{else}}false{{end}}", joinedFilter)
	if rf, ok := resolveTemplatedValue(s, tf); ok {
		b, _ := strconv.ParseBool(rf)
		return b
	}
	return false
}
