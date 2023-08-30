//go:build linux

package serializers

import (
	jlexer "github.com/mailru/easyjson/jlexer"
	jwriter "github.com/mailru/easyjson/jwriter"
)

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v EventSerializer) MarshalEasyJSON(w *jwriter.Writer) {
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *EventSerializer) UnmarshalEasyJSON(l *jlexer.Lexer) {
}