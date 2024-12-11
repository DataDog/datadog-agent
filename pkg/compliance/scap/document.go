// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The code that follows has been copied from github.com/gocomply/scap,
// which was originally released under the CC0 1.0 Universal license.

package scap

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/gocomply/scap/pkg/scap/constants"
	"github.com/gocomply/scap/pkg/scap/models/cdf"
	"github.com/gocomply/scap/pkg/scap/models/cpe_dict"
	"github.com/gocomply/scap/pkg/scap/models/ds"
	"github.com/gocomply/scap/pkg/scap/models/inter"
	"github.com/gocomply/scap/pkg/scap/models/oval_def"
	"github.com/gocomply/scap/pkg/scap/models/oval_res"
	"github.com/gocomply/scap/pkg/scap/models/oval_sc"
)

const (
	xccdfBenchmarkElement            = "Benchmark"
	cpeCpeListElement                = "cpe-list"
	ovalDefinitionsElement           = "oval_definitions"
	ovalResultsElement               = "oval_results"
	ovalSystemCharacteristicsElement = "oval_system_characteristics"
	dsDataStreamCollectionElement    = "data-stream-collection"
	ocilOcilElement                  = "ocil"
)

// Document contains all the returned informations of an openscap evaluation.
type Document struct {
	Type constants.DocumentType `json:"-"`
	*cdf.Benchmark
	*cpe_dict.CpeList
	*oval_def.OvalDefinitions
	*oval_res.OvalResults
	*oval_sc.OvalSystemCharacteristics
	*ds.DataStreamCollection
	*inter.Ocil
}

// ReadDocument takes an io.Reader and return a Document or error if failed.
func ReadDocument(r io.Reader) (*Document, error) {
	d := xml.NewDecoder(r)
	for {
		token, err := d.Token()
		if err != nil || token == nil {
			return nil, fmt.Errorf("Could not decode XML: %v", err)
		}
		switch startElement := token.(type) {
		case xml.StartElement:
			switch startElement.Name.Local {
			case dsDataStreamCollectionElement:
				var sds ds.DataStreamCollection
				if err := d.DecodeElement(&sds, &startElement); err != nil {
					return nil, err
				}
				return &Document{DataStreamCollection: &sds, Type: constants.DocumentTypeSourceDataStream}, nil
			case ovalDefinitionsElement:
				var ovalDefs oval_def.OvalDefinitions
				if err := d.DecodeElement(&ovalDefs, &startElement); err != nil {
					return nil, err
				}
				return &Document{OvalDefinitions: &ovalDefs, Type: constants.DocumentTypeOvalDefinitions}, nil
			case ovalSystemCharacteristicsElement:
				var ovalSyschar oval_sc.OvalSystemCharacteristics
				if err := d.DecodeElement(&ovalSyschar, &startElement); err != nil {
					return nil, err
				}
				return &Document{OvalSystemCharacteristics: &ovalSyschar, Type: constants.DocumentTypeOvalSyschar}, nil
			case ovalResultsElement:
				var ovalRes oval_res.OvalResults
				if err := d.DecodeElement(&ovalRes, &startElement); err != nil {
					return nil, err
				}
				return &Document{OvalResults: &ovalRes, Type: constants.DocumentTypeOvalResults}, nil
			case xccdfBenchmarkElement:
				var bench cdf.Benchmark
				if err := d.DecodeElement(&bench, &startElement); err != nil {
					return nil, err
				}
				return &Document{Benchmark: &bench, Type: constants.DocumentTypeXccdfBenchmark}, nil
			case cpeCpeListElement:
				var cpeList cpe_dict.CpeList
				if err := d.DecodeElement(&cpeList, &startElement); err != nil {
					return nil, err
				}
				return &Document{CpeList: &cpeList, Type: constants.DocumentTypeCpeDict}, nil
			case ocilOcilElement:
				var ocil inter.Ocil
				if err := d.DecodeElement(&ocil, &startElement); err != nil {
					return nil, err
				}
				return &Document{Ocil: &ocil, Type: constants.DocumentTypeOcil}, nil
			}
		}
	}
}
