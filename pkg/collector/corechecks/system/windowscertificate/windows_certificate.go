// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowscertificate implements a windows certificate check
package windowscertificate

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unsafe"

	yy "github.com/ghodss/yaml"
	"github.com/swaggest/jsonschema-go"
	"github.com/xeipuuv/gojsonschema"
	"golang.org/x/sys/windows"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "windows_certificate"

	defaultMinCollectionInterval = 300
	defaultDaysCritical          = 7
	defaultDaysWarning           = 14

	certStoreOpenFlags = windows.CERT_SYSTEM_STORE_LOCAL_MACHINE | windows.CERT_STORE_READONLY_FLAG | windows.CERT_STORE_OPEN_EXISTING_FLAG
	certEncoding       = windows.X509_ASN_ENCODING | windows.PKCS_7_ASN_ENCODING
	cryptENotFound     = windows.Errno(windows.CRYPT_E_NOT_FOUND)
)

// Config is the configuration options for this check
// it is exported so that the yaml parser can read it.
type Config struct {
	CertificateStore string   `yaml:"certificate_store" json:"certificate_store" required:"true" nullable:"false"`
	CertSubjects     []string `yaml:"certificate_subjects" json:"certificate_subjects" nullable:"false"`
	DaysCritical     int      `yaml:"days_critical" json:"days_critical" minimum:"0"`
	DaysWarning      int      `yaml:"days_warning" json:"days_warning" minimum:"0"`
}

// WinCertChk is the object representing the check
type WinCertChk struct {
	core.CheckBase
	config Config
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &WinCertChk{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, time.Duration(defaultMinCollectionInterval)*time.Second),
	}
}

func createConfigSchema() ([]byte, error) {
	reflector := jsonschema.Reflector{}
	schema, err := reflector.Reflect(Config{})
	if err != nil {
		return nil, err
	}

	schemaString, err := json.MarshalIndent(schema, "", " ")
	if err != nil {
		return nil, err
	}

	return schemaString, nil
}

// Configure accepts configuration
func (w *WinCertChk) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	w.BuildID(integrationConfigDigest, data, initConfig)
	err := w.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}

	schemaString, err := createConfigSchema()
	if err != nil {
		return fmt.Errorf("failed to create config validationschema: %s", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaString)
	rawDocument, err := yy.YAMLToJSON(data)
	if err != nil {
		log.Errorf("failed to load the config to JSON: %s", err)
		return err
	}
	documentLoader := gojsonschema.NewBytesLoader(rawDocument)
	result, _ := gojsonschema.Validate(schemaLoader, documentLoader)
	if !result.Valid() {
		for _, err := range result.Errors() {
			if err.Value() != nil {
				log.Errorf("configuration error: %s", err)
			} else {
				log.Errorf("configuration error: %s (%v)", err, err.Value())
			}
		}
		return fmt.Errorf("configuration validation failed")
	}

	config := Config{
		DaysCritical: defaultDaysCritical,
		DaysWarning:  defaultDaysWarning,
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("cannot unmarshal configuration: %s", err)
	}
	w.config = config

	if w.config.DaysWarning < w.config.DaysCritical {
		log.Warnf("Days warning (%d) is less than days critical (%d). Warning service checks will not be emitted.", w.config.DaysWarning, w.config.DaysCritical)
	}

	if w.config.DaysWarning == w.config.DaysCritical {
		log.Warnf("Days warning (%d) is equal to days critical (%d). Warning service checks will not be emitted.", w.config.DaysWarning, w.config.DaysCritical)
	}

	log.Infof("Windows Certificate Check configured with Certificate Store: '%s' and Certificate Subjects: '%v'", w.config.CertificateStore, strings.Join(w.config.CertSubjects, ", "))
	return nil
}

// Run is called each time the scheduler runs this particular check.
func (w *WinCertChk) Run() error {
	sender, err := w.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	certificates, err := getCertificates(w.config.CertificateStore, w.config.CertSubjects)
	if err != nil {
		return err
	}
	if len(certificates) == 0 {
		log.Warnf("No certificates found in store: %s for subject filters: '%s'", w.config.CertificateStore, strings.Join(w.config.CertSubjects, ", "))
	}

	for _, cert := range certificates {
		log.Debugf("Found certificate: %s", cert.Subject.String())
		daysRemaining := getCertExpiration(cert)
		expirationDate := cert.NotAfter.Format(time.RFC3339)

		// Adding Subject and Certificate Store as tags
		tags := getSubjectTags(cert)
		tags = append(tags, "certificate_store:"+w.config.CertificateStore)

		sender.Gauge("windows_certificate.days_remaining", daysRemaining, "", tags)

		if daysRemaining <= 0 {
			sender.ServiceCheck("windows_certificate.cert_expiration",
				servicecheck.ServiceCheckCritical,
				"",
				tags,
				fmt.Sprintf("Certificate has expired. Certificate expiration date is %s", expirationDate))
		} else if daysRemaining < float64(w.config.DaysCritical) {
			sender.ServiceCheck("windows_certificate.cert_expiration",
				servicecheck.ServiceCheckCritical,
				"",
				tags,
				fmt.Sprintf("Certificate will expire in only %.2f days. Certificate expiration date is %s", daysRemaining, expirationDate))
		} else if daysRemaining < float64(w.config.DaysWarning) {
			sender.ServiceCheck("windows_certificate.cert_expiration",
				servicecheck.ServiceCheckWarning,
				"",
				tags,
				fmt.Sprintf("Certificate will expire in %.2f days. Certificate expiration date is %s", daysRemaining, expirationDate))
		} else {
			sender.ServiceCheck(
				"windows_certificate.cert_expiration",
				servicecheck.ServiceCheckOK,
				"",
				tags,
				"",
			)
		}
	}

	return nil
}

func getCertificates(store string, certFilters []string) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate
	storeName := windows.StringToUTF16Ptr(store)

	log.Debugf("Opening certificate store: %s", store)
	storeHandle, err := windows.CertOpenStore(
		windows.CERT_STORE_PROV_SYSTEM,
		0,
		0,
		certStoreOpenFlags,
		uintptr(unsafe.Pointer(storeName)))
	if err != nil {
		log.Errorf("Error opening certificate store %s: %v", store, err)
		return nil, err
	}
	// Close the store when the function returns
	defer func() {
		err = windows.CertCloseStore(storeHandle, 0)
		if err != nil {
			log.Errorf("Error closing certificate store %s: %v", store, err)
		}
	}()

	log.Debugf("Enumerating certificates in store")

	if len(certFilters) == 0 {
		certificates, err = getEnumCertificatesInStore(storeHandle)
	} else {
		certificates, err = findCertificatesInStore(storeHandle, certFilters)
	}
	if err != nil {
		log.Errorf("Error getting certificates: %v", err)
		return nil, err
	}

	log.Debugf("Found %d certificates in store", len(certificates))
	return certificates, nil
}

// getEnumCertificatesInStore retrieves all certificates in a certificate store
func getEnumCertificatesInStore(storeHandle windows.Handle) ([]*x509.Certificate, error) {
	var err error
	certificates := []*x509.Certificate{}

	var certContext *windows.CertContext
	defer freeContext(certContext)

	for {
		certContext, err = windows.CertEnumCertificatesInStore(storeHandle, certContext)
		if certContext == nil {
			if err == windows.ERROR_NO_MORE_FILES {
				log.Debugf("No more certificates in store: %v", err)
				break
			} else if err == cryptENotFound {
				log.Debugf("No matching certificate found: %v", err)
				break
			} else if err != nil {
				log.Errorf("Error enumerating certificates in store: %v", err)
				return nil, err
			}
		}

		encodedCert := unsafe.Slice(certContext.EncodedCert, certContext.Length)

		cert, err := parseCertificate(encodedCert)
		if err != nil {
			log.Errorf("Error parsing certificate: %v", err)
			continue
		}

		certificates = append(certificates, cert)
	}

	return certificates, nil
}

// findCertificatesInStore finds certificates in a store with a given subject string
func findCertificatesInStore(storeHandle windows.Handle, subjectFilters []string) ([]*x509.Certificate, error) {
	var err error
	certificates := []*x509.Certificate{}

	var certContext *windows.CertContext
	defer freeContext(certContext)

	for _, subject := range subjectFilters {
		subjectName := windows.StringToUTF16Ptr(subject)

		for {
			certContext, err = windows.CertFindCertificateInStore(
				storeHandle,
				certEncoding,
				0,
				windows.CERT_FIND_SUBJECT_STR,
				unsafe.Pointer(subjectName),
				certContext,
			)
			if certContext == nil {
				if err == windows.ERROR_NO_MORE_FILES {
					log.Debugf("No more certificates in store: %v", err)
					break
				} else if err == cryptENotFound {
					log.Debugf("No matching certificate found: %v", err)
					break
				} else if err != nil {
					log.Errorf("Error enumerating certificates in store: %v", err)
					return nil, err
				}
			}

			encodedCert := unsafe.Slice(certContext.EncodedCert, certContext.Length)

			cert, err := parseCertificate(encodedCert)
			if err != nil {
				log.Errorf("Error parsing certificate: %v", err)
				continue
			}

			certificates = append(certificates, cert)
		}
	}

	return certificates, nil
}

func parseCertificate(encodedCert []byte) (*x509.Certificate, error) {
	cert, err := x509.ParseCertificate(encodedCert)
	if err != nil {
		return nil, err
	}
	log.Debugf("Parsed certificate: %s", cert.Subject.String())
	return cert, nil
}

func freeContext(certContext *windows.CertContext) {
	if certContext != nil {
		log.Debugf("Freeing certificate context")
		err := windows.CertFreeCertificateContext(certContext)
		if err != nil {
			log.Errorf("Error freeing certificate context: %v", err)
		}
	}
}

func getCertExpiration(cert *x509.Certificate) float64 {
	daysRemaining := time.Until(cert.NotAfter).Hours() / 24
	return float64(daysRemaining)
}

func getSubjectTags(cert *x509.Certificate) []string {
	subjectTags := []string{}
	subjectAttributes := cert.Subject.Names

	for _, subject := range subjectAttributes {
		subjectTags = append(subjectTags, fmt.Sprintf("subject_%s:%s", getAttributeTypeName(subject.Type.String()), subject.Value))
	}

	log.Debugf("Subject tags: %v", subjectTags)
	return subjectTags
}

// Returns the human-readable name for a given OID string
func getAttributeTypeName(oid string) string {
	switch oid {
	case "2.5.4.6":
		return "C"
	case "2.5.4.10":
		return "O"
	case "2.5.4.11":
		return "OU"
	case "2.5.4.3":
		return "CN"
	case "2.5.4.5":
		return "SERIALNUMBER"
	case "2.5.4.7":
		return "L"
	case "2.5.4.8":
		return "ST"
	case "2.5.4.9":
		return "STREET"
	case "2.5.4.17":
		return "POSTALCODE"
	case "2.5.4.12":
		return "T"
	case "2.5.4.42":
		return "GN"
	case "2.5.4.4":
		return "SN"
	case "2.5.4.46":
		return "DNQ"
	case "0.9.2342.19200300.100.1.1":
		return "UID"
	case "1.2.840.113549.1.9.1":
		return "E"
	case "0.9.2342.19200300.100.1.25":
		return "DC"
	default:
		return oid
	}
}
