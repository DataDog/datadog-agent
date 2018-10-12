// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows
// +build snmp

package network

/*
#cgo pkg-config: net-snmp-5.7.3

#include <stdlib.h>
#include <net-snmp/net-snmp-config.h>
#include <net-snmp/net-snmp-includes.h>
#include <net-snmp/mib_api.h>
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/k-sone/snmpgo"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxOIDLen      = 128
	defaultPort    = 161
	nonRepeaters   = 0
	maxRepetitions = 10
	snmpCheckName  = "snmp"
)

var once sync.Once

type metricTag struct {
	Tag    string `yaml:"tag"`
	ColOID string `yaml:"tag_oid,omitempty"`
	Column string `yaml:"column,omitempty"`
	Index  int    `yaml:"index,omitempty"`
}

type metric struct {
	MIB        string      `yaml:"MIB,omitempty"`
	OID        string      `yaml:"OID,omitempty"`
	Symbol     string      `yaml:"symbol,omitempty"`
	Name       string      `yaml:"name,omitempty"`
	Table      string      `yaml:"table,omitempty"`
	ForcedType string      `yaml:"forced_type,omitempty"`
	Symbols    []string    `yaml:"symbols,omitempty"`
	Tags       []metricTag `yaml:"metric_tags,omitempty"`
}

type snmpInstanceCfg struct {
	Host            string                  `yaml:"ip_address"`
	Port            uint                    `yaml:"port"`
	User            string                  `yaml:"user,omitempty"`
	Community       string                  `yaml:"community_string,omitempty"`
	Version         int                     `yaml:"snmp_version,omitempty"`
	AuthKey         string                  `yaml:"authKey,omitempty"`
	PrivKey         string                  `yaml:"privKey,omitempty"`
	AuthProtocol    string                  `yaml:"authProtocol,omitempty"`
	PrivProtocol    string                  `yaml:"privProtocol,omitempty"`
	ContextEngineId string                  `yaml:"context_engine_id,omitempty"`
	ContextName     string                  `yaml:"context_name,omitempty"`
	Timeout         uint                    `yaml:"timeout,omitempty"`
	Retries         uint                    `yaml:"retries,omitempty"`
	Metrics         []metric                `yaml:"metrics,omitempty"`
	Tags            []string                `yaml:"tags,omitempty"`
	OIDTranslator   *util.BiMap             `yaml:",omitempty"` //will not be in yaml
	NameLookup      map[string]string       `yaml:",omitempty"` //will not be in yaml
	MetricMap       map[string]*metric      `yaml:",omitempty"` //will not be in yaml
	TagMap          map[string][]*metricTag `yaml:",omitempty"` //will not be in yaml
	snmp            *snmpgo.SNMP
}

type snmpInitCfg struct {
	MibsDir         string `yaml:"mibs_folder,omitempty"`
	IgnoreNonIncOID string `yaml:"ignore_nonincreasing_oid,omitempty"`
}

type snmpConfig struct {
	instance snmpInstanceCfg
	initConf snmpInitCfg
}

// SNMPCheck grabs SNMP metrics
type SNMPCheck struct {
	core.CheckBase
	cfg *snmpConfig
}

func initCNetSnmpLib(cfg *snmpInitCfg) (err error) {
	err = nil

	defer func() {
		if r := recover(); r != nil {
			log.Debugf("error initializing SNMP libraries: %v", r)
			log.Debug("are all dependencies available?")
			err = errors.New("Unable to initialize SNMP")
		}
	}()

	once.Do(func() {
		C.netsnmp_init_mib()

		if cfg != nil && cfg.MibsDir != "" {
			_, e := os.Stat(cfg.MibsDir)
			if e == nil || !os.IsNotExist(e) {
				mibdir := C.CString(cfg.MibsDir)
				defer C.free(unsafe.Pointer(mibdir))
				C.add_mibdir(mibdir)
			}
		}
	})

	return
}

type tagRegistryEntry struct {
	Varbinds snmpgo.VarBinds
	Tag      *metricTag
}

type tagRegistry map[string]tagRegistryEntry

func namespaceMetric(name string) string {
	var buffer bytes.Buffer

	//build correct metric name
	buffer.WriteString("snmp.")

	tokenized := strings.Split(name, "::")
	buffer.WriteString(tokenized[len(tokenized)-1])
	return buffer.String()
}

func buildTextOID(oids []string) string {
	var buffer bytes.Buffer
	for idx, oid := range oids {
		buffer.WriteString(oid)
		if idx < len(oids)-1 {
			buffer.WriteString("::")
		}
	}
	return buffer.String()
}

func buildOID(holder []C.oid, idLen int) string {
	var buffer bytes.Buffer
	for i := 0; i < idLen; i++ {
		buffer.WriteString(fmt.Sprintf("%v", holder[i]))
		if i < (idLen - 1) {
			buffer.WriteString(".")
		}
	}
	return buffer.String()
}

func getTextualOID(oid string) (string, error) {
	var cbuff [512]C.char
	var oidHolder [maxOIDLen]C.oid

	//zero out buffer
	for i := range cbuff {
		cbuff[i] = 0
	}
	_oid := strings.Split(oid, ".")
	if _oid[0] == "." {
		_oid = _oid[1:]
	}

	for i := range _oid {
		id, err := strconv.Atoi(_oid[i])
		if err != nil {
			return "", err
		}
		oidHolder[i] = (C.oid)(id)
	}
	idLen := len(_oid)
	cbuffLen := 511

	C.snprint_objid(
		&cbuff[0],
		(C.size_t)(cbuffLen),
		(*C.oid)(unsafe.Pointer(&oidHolder[0])),
		(C.size_t)(idLen))

	return C.GoString((*C.char)(unsafe.Pointer(&cbuff[0]))), nil

}

func getIndexTag(baseOID, OID string, idx int) (string, error) {

	_base := strings.Split(baseOID, ".")
	if _base[0] == "." {
		_base = _base[1:]
	}

	_oid := strings.Split(OID, ".")
	if _oid[0] == "." {
		_oid = _oid[1:]
	}
	if len(_oid) < len(_base)+idx {
		return "", errors.New("index provided unavailable in target OID")
	}
	_oid = _oid[:len(_base)+idx]

	textOID, err := getTextualOID(strings.Join(_oid, "."))
	if err != nil {
		return "", err
	}
	sliceOID := strings.Split(textOID, ".")
	tag := sliceOID[len(sliceOID)-1]

	return tag, nil
}

func (cfg *snmpInstanceCfg) generateOIDs() error {

	var textualOID string
	var err error
	cfg.OIDTranslator = util.NewBiMap((string)(""), (string)(""))
	cfg.MetricMap = make(map[string]*metric)
	cfg.NameLookup = make(map[string]string)

	for i, metric := range cfg.Metrics {
		oidHolder := make([]C.oid, maxOIDLen)
		idLen := maxOIDLen

		log.Debugf("Mapping metric: %v", metric)
		if metric.OID != "" {
			log.Debugf("No translation necessary for OID: %s", metric.OID)
			err = cfg.OIDTranslator.AddKV(metric.OID, metric.OID)
			if err != nil {
				log.Warnf("Unable to add OID %s to translation map", metric.OID)
			}
			cfg.MetricMap[metric.OID] = &cfg.Metrics[i]
		} else if metric.Table != "" {
			for _, symbol := range metric.Symbols {
				textualOID = buildTextOID([]string{metric.MIB, symbol})
				ctextualOID := C.CString(textualOID)
				log.Debugf("Translating Table OID: %s", textualOID)

				C.read_objid(
					ctextualOID,
					(*C.oid)(unsafe.Pointer(&oidHolder[0])),
					(*C.size_t)(unsafe.Pointer(&idLen)))

				symOID := buildOID(oidHolder, idLen)
				err = cfg.OIDTranslator.AddKV(textualOID, symOID)
				if err != nil {
					log.Warnf("Unable to add OID %s to translation map", textualOID)
				}
				cfg.MetricMap[symOID] = &cfg.Metrics[i]
				cfg.NameLookup[textualOID] = namespaceMetric(textualOID)
				C.free(unsafe.Pointer(ctextualOID))
			}
		} else {
			textualOID = buildTextOID([]string{metric.MIB, metric.Symbol})
			ctextualOID := C.CString(textualOID)
			log.Debugf("Translating Symbol: %s", textualOID)

			C.read_objid(
				ctextualOID,
				(*C.oid)(unsafe.Pointer(&oidHolder[0])),
				(*C.size_t)(unsafe.Pointer(&idLen)))

			symOID := buildOID(oidHolder, idLen)
			err = cfg.OIDTranslator.AddKV(textualOID, symOID)
			if err != nil {
				log.Warnf("Unable to add OID %s to translation map", textualOID)
			}
			cfg.MetricMap[symOID] = &cfg.Metrics[i]
			cfg.NameLookup[textualOID] = namespaceMetric(textualOID)
			C.free(unsafe.Pointer(ctextualOID))
		}

		//grab tags too
		for _, tag := range metric.Tags {
			if tag.Column != "" {
				textualTagOID := buildTextOID([]string{metric.MIB, tag.Column})
				ctextualOID := C.CString(textualTagOID)
				log.Debugf("Translating Tag OID: %s", textualTagOID)

				C.read_objid(
					ctextualOID,
					(*C.oid)(unsafe.Pointer(&oidHolder[0])),
					(*C.size_t)(unsafe.Pointer(&idLen)))

				err = cfg.OIDTranslator.AddKV(textualTagOID, buildOID(oidHolder, idLen))
				if err != nil {
					log.Warnf("Unable to add OID %s to translation map", textualTagOID)
					C.free(unsafe.Pointer(ctextualOID))
					continue // or bail out?
				}
				C.free(unsafe.Pointer(ctextualOID))
			}
		}
	}

	return nil
}

func (cfg *snmpInstanceCfg) generateTagMap() error {

	cfg.TagMap = make(map[string][]*metricTag)

	for _, metric := range cfg.Metrics {

		var ok bool
		var textOID, symOID string

		for idx, tag := range metric.Tags {
			textOID = ""
			symOID = ""

			//Set column ID for lookup if it applies
			if tag.Column != "" {
				textOID = buildTextOID([]string{metric.MIB, tag.Column})
				OID, err := cfg.OIDTranslator.GetKV(textOID)
				if err != nil {
					continue
				}
				symOID, ok = OID.(string)
				if !ok {
					continue
				}

				metric.Tags[idx].ColOID = string(symOID)
			}

			if metric.OID != "" {
				if _, ok := cfg.TagMap[metric.OID]; ok {
					cfg.TagMap[metric.OID] = append(cfg.TagMap[metric.OID], &metric.Tags[idx])
				} else {
					cfg.TagMap[metric.OID] = []*metricTag{&metric.Tags[idx]}
				}
			} else if metric.Table != "" {
				for _, symbol := range metric.Symbols {
					metricOID := buildTextOID([]string{metric.MIB, symbol})

					OID, err := cfg.OIDTranslator.GetKV(metricOID)
					if err != nil {
						continue
					}
					symMetricOID, ok := OID.(string)
					if !ok {
						continue
					}
					if _, ok := cfg.TagMap[symMetricOID]; ok {
						cfg.TagMap[symMetricOID] = append(cfg.TagMap[symMetricOID], &metric.Tags[idx])
					} else {
						cfg.TagMap[symMetricOID] = []*metricTag{&metric.Tags[idx]}
					}
				}
			} else {
				metricOID := buildTextOID([]string{metric.MIB, metric.Symbol})

				OID, err := cfg.OIDTranslator.GetKV(metricOID)
				if err != nil {
					continue
				}
				symMetricOID, ok := OID.(string)
				if !ok {
					continue
				}
				if _, ok := cfg.TagMap[symMetricOID]; ok {
					cfg.TagMap[symMetricOID] = append(cfg.TagMap[symMetricOID], &metric.Tags[idx])
				} else {
					cfg.TagMap[symMetricOID] = []*metricTag{&metric.Tags[idx]}
				}
			}
		}
	}

	return nil
}

func (c *snmpConfig) parse(data []byte, initData []byte) error {
	var tagbuff bytes.Buffer
	var instance snmpInstanceCfg
	var initConf snmpInitCfg

	if err := yaml.Unmarshal(data, &instance); err != nil {
		return err
	}
	if err := yaml.Unmarshal(initData, &initConf); err != nil {
		return err
	}

	c.instance = instance
	c.initConf = initConf

	if c.instance.Port == 0 {
		c.instance.Port = defaultPort
	}

	if c.instance.Host == "" {
		return errors.New("error parsing configuration - invalid SNMP instance configuration")
	}

	//build instance tag
	tagbuff.Reset()
	tagbuff.WriteString("snmp_device:")
	tagbuff.WriteString(c.instance.Host)
	tagbuff.WriteString(":")
	tagbuff.WriteString(fmt.Sprintf("%d", c.instance.Port))

	c.instance.Tags = append(c.instance.Tags, tagbuff.String())

	//security - make sure we're backward compatible
	switch c.instance.AuthProtocol {
	case "usmHMACMD5AuthProtocol":
		c.instance.AuthProtocol = string(snmpgo.Md5)
	case "usmHMACSHAAuthProtocol":
		c.instance.AuthProtocol = string(snmpgo.Sha)
	}

	switch c.instance.PrivProtocol {
	case "usmDESPrivProtocol", "usm3DESEDEPrivProtocol":
		c.instance.PrivProtocol = string(snmpgo.Des)
	case "usmAesCfb128Protocol", "usmAesCfb192Protocol", "usmAesCfb256Protocol":
		c.instance.PrivProtocol = string(snmpgo.Aes)
	}

	// if version not set explicitly - infer from params.
	if c.instance.Version == 0 && c.instance.User != "" {
		c.instance.Version = 3
	}

	return nil
}

func (c *SNMPCheck) submitSNMP(oids snmpgo.Oids, vbs snmpgo.VarBinds) error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	for _, oid := range oids {
		varbinds := vbs.MatchBaseOids(oid)

		registry := make(tagRegistry)
		if tags, ok := c.cfg.instance.TagMap[oid.String()]; ok {
			for _, tag := range tags {
				if tag.Column != "" {
					if tagOID, err := snmpgo.NewOid(tag.ColOID); err == nil {
						registry[tag.Tag] = tagRegistryEntry{Tag: tag, Varbinds: vbs.MatchBaseOids(tagOID)}
					}
				} else if tag.Index > 0 {
					registry[tag.Tag] = tagRegistryEntry{Tag: tag, Varbinds: nil}
				}
			}
		}

		symbolicOID, err := c.cfg.instance.OIDTranslator.GetKVReverse(oid.String())
		if err != nil {
			log.Warnf("unable to report OID: %s - error retrieving value.", oid.String())
			continue
		}

		//forced type?
		metricName := ""
		metricType := ""
		if m, ok := c.cfg.instance.MetricMap[oid.String()]; ok {
			if m.ForcedType != "" {
				log.Warnf("Detected forced type: %s - for %v.", m.ForcedType, *m)
				metricType = m.ForcedType
			}

			if m.Name != "" {
				metricName = namespaceMetric(m.Name)
				log.Debugf("Overriding name: %s - for %v.", metricName, *m)
			}
		}

		if metric, ok := symbolicOID.(string); ok {
			for idx, collected := range varbinds {
				var tag string
				var tagbuff bytes.Buffer
				var value *big.Int
				var err error

				value, err = collected.Variable.BigInt()
				if err != nil || value == nil {
					continue
				}

				//set tag
				tagbundle := append([]string(nil), c.cfg.instance.Tags...)
				for _, entry := range registry {
					tagbuff.Reset()
					if entry.Tag.Column != "" && len(entry.Varbinds) > 0 {
						tagbuff.WriteString(entry.Tag.Tag)
						tagbuff.WriteString(":")
						tagbuff.WriteString(entry.Varbinds[idx].Variable.String())
						tagbundle = append(tagbundle, tagbuff.String())
					} else if entry.Tag.Index > 0 {
						tag, _ = getIndexTag(oid.String(), collected.Oid.String(), entry.Tag.Index)
						tagbuff.WriteString(entry.Tag.Tag)
						tagbuff.WriteString(":")
						tagbuff.WriteString(tag)
						tagbundle = append(tagbundle, tagbuff.String())
					}
				}

				//TODO: add more types
				log.Debugf("Variable fetched has type: %v = %v", metric, collected.Variable.Type())
				if metricType == "" {
					metricType = collected.Variable.Type()
				}
				if metricName == "" {
					if ddMetric, ok := c.cfg.instance.NameLookup[metric]; ok {
						metricName = ddMetric
					} else {
						metricName = namespaceMetric(metric)
					}
				}

				switch metricType {
				case "Gauge32", "Integer", "gauge":
					log.Debugf("Submitting gauge: %v = %v tagged with: %v", metricName, value, tagbundle)
					//should report instance has hostname
					sender.Gauge(metricName, float64(value.Int64()), "", tagbundle)
				case "Counter64", "Counter32":
					log.Debugf("Submitting rate: %v = %v tagged with: %v", metricName, value, tagbundle)
					//should report instance has hostname
					sender.Rate(metricName, float64(value.Int64()), "", tagbundle)
				case "OctetString":
				default:
					continue
				}
			}
		}
	}

	sender.Commit()

	return nil
}

func (c *SNMPCheck) getSNMP() error {

	// get OIDList
	oidvalues := c.cfg.instance.OIDTranslator.Values()
	oidList := make([]string, len(oidvalues))
	for i, v := range oidvalues {
		//should be true for each v
		if vstr, ok := v.(string); ok {
			oidList[i] = vstr
		}
	}

	oids, err := snmpgo.NewOids(oidList)
	if err != nil {
		// Failed creating Native OID list.
		log.Warnf("Error creating Native OID list: %v", err)
		return err
	}

	log.Debugf("Connecting to SNMP host...")
	if err = c.cfg.instance.snmp.Open(); err != nil {
		// Failed to open connection
		log.Warnf("Error connecting to host: %v", err)
		return err
	}

	log.Debugf("SNMP Getting... OIDS: %v", oids)
	pdu, err := c.cfg.instance.snmp.GetRequest(oids)
	if err != nil {
		// Failed to request
		log.Warnf("Error performing snmpget: %v", err)
		return err
	}
	if pdu.ErrorStatus() != snmpgo.NoError {
		// Received an error from the agent
		log.Warnf("Received error from SNMP agent: %v - %v", pdu.ErrorStatus(), pdu.ErrorIndex())
	}

	var collectedVars []*snmpgo.VarBind
	var missingOIDs []*snmpgo.Oid
	for _, oid := range oids {
		collected := pdu.VarBinds().MatchOid(oid)
		if collected != nil && collected.Variable.Type() != "NoSucheInstance" {
			log.Debugf("Collected OID: %v", oid)
			collectedVars = append(collectedVars, collected)
		} else {
			log.Debugf("Missing OID: %v", oid)
			missingOIDs = append(missingOIDs, oid)
		}
	}

	if len(missingOIDs) > 0 {
		log.Debugf("SNMP Walking...")
		// See: https://www.snmpsharpnet.com/?page_id=30
		pdu, err := c.cfg.instance.snmp.GetBulkWalk(missingOIDs, nonRepeaters, maxRepetitions)
		if err != nil {
			// Failed to request
			log.Warnf("Error performing snmpget: %v", err)
			return err
		}
		if pdu.ErrorStatus() != snmpgo.NoError {
			// Received an error from the agent
			log.Warnf("Received error from SNMP agent: %v - %v", pdu.ErrorStatus(), pdu.ErrorIndex())
		}

		for _, oid := range oids {
			varbinds := pdu.VarBinds().MatchBaseOids(oid)
			for _, collected := range varbinds {
				if collected != nil && collected.Variable.Type() != "NoSucheInstance" {
					log.Debugf("Collected OID: %v", collected.Oid.String())
					collectedVars = append(collectedVars, collected)
				}
			}
		}
	}

	log.Debugf("Submitting metrics...")
	c.submitSNMP(oids, collectedVars)

	//TODO: send service checks
	return nil
}

// Configure the check from YAML data
func (c *SNMPCheck) Configure(data integration.Data, initConfig integration.Data) error {
	err := c.CommonConfigure(data)
	if err != nil {
		return err
	}

	cfg := new(snmpConfig)
	err = cfg.parse(data, initConfig)
	if err != nil {
		log.Criticalf("Error parsing configuration file: %s ", err)
		return err
	}
	c.BuildID(data, initConfig)
	c.cfg = cfg

	//init SNMP - will fail if missing snmp libs.
	if err = initCNetSnmpLib(&c.cfg.initConf); err != nil {
		log.Criticalf("Unable to configure check: %s ", err)
		return err
	}

	//create snmp object for instance
	snmpver := snmpgo.V2c
	if c.cfg.instance.Version == 3 {
		snmpver = snmpgo.V3
	}

	//sec level
	seclevel := snmpgo.NoAuthNoPriv
	if snmpver == snmpgo.V3 && c.cfg.instance.AuthKey != "" {
		if c.cfg.instance.PrivKey != "" {
			seclevel = snmpgo.AuthPriv
		} else {
			seclevel = snmpgo.AuthNoPriv
		}
	}

	c.cfg.instance.snmp, err = snmpgo.NewSNMP(snmpgo.SNMPArguments{
		Version:         snmpver,
		Address:         net.JoinHostPort(c.cfg.instance.Host, strconv.Itoa(int(c.cfg.instance.Port))),
		Retries:         c.cfg.instance.Retries,
		Timeout:         time.Duration(c.cfg.instance.Timeout) * time.Second,
		UserName:        c.cfg.instance.User,
		Community:       c.cfg.instance.Community,
		AuthPassword:    c.cfg.instance.AuthKey,
		AuthProtocol:    snmpgo.AuthProtocol(c.cfg.instance.AuthProtocol),
		PrivPassword:    c.cfg.instance.PrivKey,
		PrivProtocol:    snmpgo.PrivProtocol(c.cfg.instance.PrivProtocol),
		ContextEngineId: c.cfg.instance.ContextEngineId,
		ContextName:     c.cfg.instance.ContextName,
		SecurityLevel:   seclevel,
	})
	if err != nil {
		// Failed to create snmpgo.SNMP object
		log.Warnf("Error creating SNMP instance for: %s (%v) - skipping", c.cfg.instance.Host, err)
		return err
	}

	// genereate OID Translator and TagMap - one time thing
	if c.cfg.instance.OIDTranslator == nil {
		log.Debugf("Generating OIDs...")
		if err := c.cfg.instance.generateOIDs(); err != nil {
			return err
		}
	}
	if c.cfg.instance.TagMap == nil {
		log.Debugf("Generating TagMap...")
		if err := c.cfg.instance.generateTagMap(); err != nil {
			return err
		}
	}

	return nil
}

// Run runs the check
func (c *SNMPCheck) Run() error {
	log.Debugf("Grabbing SNMP variables...")
	if err := c.getSNMP(); err != nil {
		return err
	}

	return nil
}

func snmpFactory() check.Check {
	return &SNMPCheck{
		CheckBase: core.NewCheckBase(snmpCheckName),
	}
}

func init() {
	core.RegisterCheck(snmpCheckName, snmpFactory)
}
