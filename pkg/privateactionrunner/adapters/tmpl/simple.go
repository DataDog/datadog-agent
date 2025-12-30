// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tmpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/aymerick/raymond/lexer"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type config struct {
	preservePathRoots []string
}

func (c *config) shouldPreserveExpr(path []string) bool {
	root := path[0]
	for _, r := range c.preservePathRoots {
		if r == root {
			return true
		}
	}
	return false
}

type Option func(c *config)

func PreserveExpressionsWithPathRoots(roots ...string) Option {
	return func(c *config) {
		c.preservePathRoots = append(c.preservePathRoots, roots...)
	}
}

type ErrPathNotFound struct {
	FullyQualifiedPath string
	Context            interface{}
}

func (err ErrPathNotFound) Error() string {
	return fmt.Sprintf("\"%s\" not found in %v", err.FullyQualifiedPath, err.Context)
}

type ParseError struct {
	Description string
	Pos         int
}

func (err ParseError) Error() string {
	return fmt.Sprintf("(pos %d) %s", err.Pos, err.Description)
}

func unexpectedTokenError(t lexer.Token) ParseError {
	return ParseError{Description: fmt.Sprintf("Unexpected token: '%s'", t.Val), Pos: t.Pos}
}

type Template struct {
	fragments []fragment
}

type fragment interface {
	render(input interface{}) (string, error)
}

type stringFragment struct {
	text string
}

func (sf stringFragment) render(_ interface{}) (string, error) {
	return sf.text, nil
}

type expressionFragment struct {
	path []string
}

func (ef expressionFragment) render(input interface{}) (string, error) {
	val, err := evaluatePath(input, ef.path)
	var epnf ErrPathNotFound
	if errors.As(err, &epnf) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return stringify(val)
}

func parsePath(pathExpr string) ([]string, error) {
	tmpl, err := Parse(fmt.Sprintf("{{ %s }}", pathExpr))
	if err == nil {
		if len(tmpl.fragments) == 1 {
			if frag, ok := tmpl.fragments[0].(expressionFragment); ok {
				return frag.path, nil
			}
		}
	}

	return nil, fmt.Errorf("not a valid path expression: \"%s\"", pathExpr)
}

func MustEvaluatePath(input interface{}, pathExpr string) interface{} {
	res, err := EvaluatePath(input, pathExpr)
	if err != nil {
		panic(err)
	}
	return res
}

func EvaluatePath(input interface{}, pathExpr string) (interface{}, error) {
	path, err := parsePath(pathExpr)
	if err != nil {
		return nil, err
	}
	return evaluatePath(input, path)
}

func evaluatePath(input interface{}, path []string) (interface{}, error) {
	inputVal := reflect.ValueOf(input)
	return evaluateExpression(path, 0, inputVal, inputVal)
}

func stringify(data interface{}) (string, error) {
	val := reflect.ValueOf(data)
	if !val.IsValid() {
		return "", nil
	}
	switch val.Kind() {
	case reflect.Map, reflect.Struct, reflect.Array, reflect.Slice:
		marshaled, err := jsonMarshalWithoutEscaping(val.Interface())
		if err != nil {
			return "", err
		}
		return string(marshaled), nil
	case reflect.Int:
		return stringifyInt64(int64(val.Interface().(int))), nil
	case reflect.Int8:
		return stringifyInt64(int64(val.Interface().(int8))), nil
	case reflect.Int16:
		return stringifyInt64(int64(val.Interface().(int16))), nil
	case reflect.Int32:
		return stringifyInt64(int64(val.Interface().(int32))), nil
	case reflect.Int64:
		return stringifyInt64(val.Interface().(int64)), nil
	case reflect.Uint:
		return stringifyUint64(uint64(val.Interface().(uint))), nil
	case reflect.Uint8:
		return stringifyUint64(uint64(val.Interface().(uint8))), nil
	case reflect.Uint16:
		return stringifyUint64(uint64(val.Interface().(uint16))), nil
	case reflect.Uint32:
		return stringifyUint64(uint64(val.Interface().(uint32))), nil
	case reflect.Uint64:
		return stringifyUint64(val.Interface().(uint64)), nil
	case reflect.Float32:
		return stringifyFloat64(float64(val.Interface().(float32))), nil
	case reflect.Float64:
		return stringifyFloat64(val.Interface().(float64)), nil
	}
	return fmt.Sprintf("%v", val.Interface()), nil
}

// Calls SetEscapeHTML(false) so the string is not encoded using HTMLEscape (which escapes "<", ">", "&", U+2028, and
// U+2029 to "\u003c","\u003e", "\u0026", "\u2028", and "\u2029"). See https://pkg.go.dev/encoding/json#Marshal.
func jsonMarshalWithoutEscaping(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(t)
	// Encoding adds a new line character at the end. Remove this and return.
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), err
}

func stringifyInt64(val int64) string {
	return strconv.FormatInt(val, 10)
}

func stringifyUint64(val uint64) string {
	return strconv.FormatUint(val, 10)
}

func stringifyFloat64(val float64) string {
	return strconv.FormatFloat(val, 'f', -1, 64)
}

func evaluateExpression(path []string, i int, input, topLevelInput reflect.Value) (interface{}, error) {
	var val reflect.Value
	part := path[i]
	// "[foo bar]" => "foo bar"
	if (len(part) >= 2) && (part[0] == '[') && (part[len(part)-1] == ']') {
		part = part[1 : len(part)-1]
	}
	switch input.Kind() {
	case reflect.Ptr:
		return evaluateExpression(path, i, input.Elem(), topLevelInput)
	case reflect.Struct:
		caser := cases.Title(language.English)
		expFieldName := caser.String(part)
		// check if struct have this field and that it is exported
		if tField, ok := input.Type().FieldByName(expFieldName); ok && (tField.PkgPath == "") {
			// struct field
			val = input.FieldByIndex(tField.Index)
		}
	case reflect.Map:
		nameVal := reflect.ValueOf(part)
		if nameVal.Type().AssignableTo(input.Type().Key()) {
			// map key
			val = input.MapIndex(nameVal)
		}
	case reflect.Array, reflect.Slice:
		if i, err := strconv.Atoi(part); (err == nil) && (i > -1) && (i < input.Len()) {
			val = input.Index(i)
		}
	case reflect.Interface:
		return evaluateExpression(path, i, input.Elem(), topLevelInput)
	}
	if !val.IsValid() {
		return nil, ErrPathNotFound{
			FullyQualifiedPath: strings.Join(path[0:i+1], "."),
			Context:            topLevelInput,
		}
	}

	if i == len(path)-1 {
		for val.Kind() == reflect.Interface {
			val = val.Elem()
		}
		if !val.IsValid() {
			return nil, nil
		}

		switch val.Kind() {
		case reflect.Chan, reflect.Func:
			return nil, ErrPathNotFound{
				FullyQualifiedPath: strings.Join(path[0:i+1], "."),
				Context:            topLevelInput,
			}
		}
		return val.Interface(), nil
	}
	return evaluateExpression(path, i+1, val, topLevelInput)
}

func Parse(tmpl string, opts ...Option) (*Template, error) {
	conf := &config{}
	for _, o := range opts {
		o(conf)
	}

	lex := lexer.Scan(tmpl)
	template := Template{}
	previousIsSep := false
	done := false

	defer func() {
		if done {
			return
		}
		go func() {
			for {
				token := lex.NextToken()
				if token.Kind == lexer.TokenEOF || token.Kind == lexer.TokenError {
					break
				}
			}
		}()
	}()

	var path []string
	for !done {
		token := lex.NextToken()
		switch token.Kind {
		case lexer.TokenEOF:
			done = true
		case lexer.TokenContent:
			template.fragments = append(template.fragments, stringFragment{text: token.Val})
		case lexer.TokenOpen:
		case lexer.TokenSep:
			if len(path) == 0 {
				return nil, unexpectedTokenError(token)
			}
			if token.Val == "/" {
				return nil, unexpectedTokenError(token)
			}
		case lexer.TokenClose:
			if len(path) == 0 {
				return nil, unexpectedTokenError(token)
			}
			if conf.shouldPreserveExpr(path) {
				template.fragments = append(template.fragments, stringFragment{text: fmt.Sprintf("{{ %s }}", strings.Join(path, "."))})
			} else {
				template.fragments = append(template.fragments, expressionFragment{path: path})
			}
			path = nil
		case lexer.TokenID:
			if token.Val == ".." {
				return nil, unexpectedTokenError(token)
			}
			if len(path) > 0 && !previousIsSep {
				return nil, unexpectedTokenError(token)
			}
			path = append(path, token.Val)
		case lexer.TokenError:
			done = true
			return nil, ParseError{Description: token.Val, Pos: token.Pos}
		default:
			return nil, unexpectedTokenError(token)
		}
		previousIsSep = token.Kind == lexer.TokenSep
	}

	return &template, nil
}

func (t *Template) Render(input interface{}) (string, error) {
	var sb strings.Builder
	for _, frag := range t.fragments {
		rendered, err := frag.render(input)
		if err != nil {
			return "", err
		}
		sb.WriteString(rendered)
	}
	return sb.String(), nil
}

func ParseAndRender(tmpl string, input interface{}, opts ...Option) (string, error) {
	template, err := Parse(tmpl, opts...)
	if err != nil {
		return "", err
	}
	return template.Render(input)
}
