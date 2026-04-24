// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package xmlite is a fast XML decoder
package xmlite

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// Handler is an interface for handling XML decoding
type Handler interface {
	StartTag(name []byte)
	EndTag(name []byte)
	Attr(name, value []byte)
	CharData(value []byte)
}

// LiteDecoder is an efficient implementation of an XML decoder
type LiteDecoder struct {
	reader    io.ByteReader
	handler   Handler
	peekStore int
	nameBuff  bytes.Buffer
	attrBuff  bytes.Buffer
	buff      bytes.Buffer
}

// NewLiteDecoder creates a LiteDecoder
func NewLiteDecoder(reader io.Reader, handler Handler) *LiteDecoder {
	lt := &LiteDecoder{
		peekStore: -1,
		handler:   handler,
	}

	if rb, ok := reader.(io.ByteReader); ok {
		lt.reader = rb
	} else {
		lt.reader = bufio.NewReader(reader)
	}
	return lt
}

func (lt *LiteDecoder) getc() (byte, error) {
	if lt.peekStore >= 0 {
		val := lt.peekStore
		lt.peekStore = -1
		return byte(val), nil
	}

	return lt.reader.ReadByte()
}

func (lt *LiteDecoder) peekc() (byte, error) {
	if lt.peekStore < 0 {
		b, err := lt.reader.ReadByte()
		if err != nil {
			return b, err
		}
		lt.peekStore = int(b)
	}
	return byte(lt.peekStore), nil
}

func (lt *LiteDecoder) clearPeek() {
	lt.peekStore = -1
}

// Parse starts parsing the XML
func (lt *LiteDecoder) Parse() error {
	for {
		if err := lt.NextToken(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// NextToken advances to the next XML token
func (lt *LiteDecoder) NextToken() error {
	for {
		curr, err := lt.peekc()
		if err != nil {
			return err
		}

		if curr != '<' {
			cd, err := lt.charData()
			if err != nil {
				return err
			}
			lt.handler.CharData(cd)
			return nil
		}

		// handle tag
		lt.clearPeek()

		// possibly </ or <?
		curr1, err := lt.peekc()
		if err != nil {
			return err
		}

		isEnd := false
		if curr1 == '/' {
			isEnd = true
			lt.clearPeek()
		} else if curr1 == '?' {
			_, _ = lt.skipUntilEndTag('?')
			continue
		}

		// handle name
		name, err := lt.name(false)
		if err != nil {
			return err
		}
		if isEnd {
			lt.handler.EndTag(name)
		} else {
			lt.handler.StartTag(name)
		}

		_ = lt.space()

		// handle attributes
		if !isEnd {
			for !lt.isNextSlashOrRightOrErr() {
				attrName, err := lt.name(true)
				if err != nil {
					return err
				}
				if err := lt.eat('='); err != nil {
					return err
				}

				attrValue, err := lt.quote()
				if err != nil {
					return err
				}
				lt.handler.Attr(attrName, attrValue)

				_ = lt.space()
			}
		}

		// end tag
		autoclose, err := lt.skipUntilEndTag('/')
		if err != nil {
			return err
		}
		if autoclose {
			// name is still valid since we use a different buffer
			lt.handler.EndTag(name)
		}

		return nil
	}
}

func (lt *LiteDecoder) charData() ([]byte, error) {
	lt.buff.Reset()
	for {
		curr, err := lt.peekc()
		if err != nil {
			return nil, err
		}

		if curr == '<' {
			return lt.buff.Bytes(), nil
		}

		lt.clearPeek()
		lt.buff.WriteByte(curr)
	}
}

func (lt *LiteDecoder) skipUntilEndTag(before byte) (bool, error) {
	prev := -1
	for {
		curr, err := lt.getc()
		if err != nil {
			return false, err
		}

		if curr == '>' {
			return prev == int(before), nil
		}
		prev = int(curr)
	}
}

func (lt *LiteDecoder) isNextSlashOrRightOrErr() bool {
	next, err := lt.peekc()
	return err != nil || next == '/' || next == '>'
}

func (lt *LiteDecoder) eat(expected byte) error {
	next, err := lt.getc()
	if err != nil {
		return err
	}
	if next != expected {
		return fmt.Errorf("expected `%c` and found `%c`", expected, next)
	}
	return nil
}

func (lt *LiteDecoder) name(attr bool) ([]byte, error) {
	buff := &lt.nameBuff
	if attr {
		buff = &lt.attrBuff
	}

	buff.Reset()
	for {
		curr, err := lt.peekc()
		if err != nil {
			return nil, err
		}

		if isNameChar(curr) {
			lt.clearPeek()
			buff.WriteByte(curr)
		} else {
			return buff.Bytes(), nil
		}
	}
}

func (lt *LiteDecoder) quote() ([]byte, error) {
	delim, err := lt.getc()
	if err != nil {
		return nil, err
	}

	if delim != '\'' && delim != '"' {
		return nil, errors.New("expected quote delimiter")
	}

	lt.buff.Reset()
	for {
		curr, err := lt.getc()
		if err != nil {
			return nil, err
		}

		if curr != delim {
			lt.buff.WriteByte(curr)
		} else {
			return lt.buff.Bytes(), nil
		}
	}
}

func (lt *LiteDecoder) space() error {
	for {
		curr, err := lt.peekc()
		if err != nil {
			return err
		}

		if curr == ' ' || curr == '\t' || curr == '\n' || curr == '\r' {
			lt.clearPeek()
			continue
		}

		return nil
	}
}

func isNameChar(c byte) bool {
	return c == ':' || c == '-' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}
