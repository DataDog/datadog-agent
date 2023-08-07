// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"google.golang.org/protobuf/proto"

	languagepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/languagedetection"
)

const ContentTypeProtobuf = "application/protobuf"

var (
	pSerializer             = protoSeralizer{}
	_           Marshaler   = protoSeralizer{}
	_           Unmarshaler = protoSeralizer{}
)

type Marshaler interface {
	Marshal(req *languagepb.DetectLanguageRequest) ([]byte, error)
	ContentType() string
}

type Unmarshaler interface {
	Unmarshal(reader []byte) ([]languagemodels.Language, error)
}

type protoSeralizer struct{}

func (protoSeralizer) Marshal(req *languagepb.DetectLanguageRequest) ([]byte, error) {
	return proto.Marshal(req)
}

func (protoSeralizer) ContentType() string {
	return ContentTypeProtobuf
}

func (protoSeralizer) Unmarshal(blob []byte) ([]languagemodels.Language, error) {
	var res languagepb.DetectLanguageResponse
	err := proto.Unmarshal(blob, &res)
	if err != nil {
		return nil, err
	}

	langs := make([]languagemodels.Language, len(res.Languages))
	for i, lang := range res.Languages {
		langs[i] = languageFromRes(lang)
	}
	return langs, nil
}

func languageFromRes(res *languagepb.Language) languagemodels.Language {
	return languagemodels.Language{
		Name:    languagemodels.LanguageName(res.Name),
		Version: res.Version,
	}
}

func GetMarshaller() Marshaler {
	return pSerializer
}

func GetUnmarshaller() Unmarshaler {
	return pSerializer
}
