// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.
//go:build windows

//nolint:revive // TODO(WINA) Fix revive linter

// Package windowscertificate implements a windows certificate check
package windowscertificate

import (
	"crypto/x509"
	"fmt"
	"time"
	"unsafe"

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
)

// Config is the configuration options for this check
// it is exported so that the yaml parser can read it.
type Config struct {
	CertificateStores []string `yaml:"certificate_stores"`
	CertSubject       []string `yaml:"certificate_subject"`
	DaysCritical      int      `yaml:"days_critical"`
	DaysWarning       int      `yaml:"days_warning"`
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
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Configure accepts configuration
func (w *WinCertChk) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := w.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, &w.config); err != nil {
		return err
	}

	if w.config.DaysCritical == 0 {
		w.config.DaysCritical = 7
	}
	if w.config.DaysWarning == 0 {
		w.config.DaysWarning = 14
	}

	log.Infof("Windows Certificate Check configured with Certificate Stores: %v and Certificate Subjects: %v", w.config.CertificateStores, w.config.CertSubject)
	return nil
}

// Run is called each time the scheduler runs this particular check.
func (w *WinCertChk) Run() error {
	sender, err := w.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	if len(w.config.CertificateStores) == 0 {
		return fmt.Errorf("no certificate stores specified")
	}

	for _, store := range w.config.CertificateStores {
		certificates, err := getCertificates(store, w.config.CertSubject)
		if err != nil {
			return err
		}
		if len(certificates) == 0 {
			log.Warnf("No certificates found in store: %s for subject filters: %v", store, w.config.CertSubject)
			continue
		}

		for _, cert := range certificates {
			log.Debugf("Found certificate: %s", cert.Subject.String())
			daysRemaining := getCertExpiration(cert)

			// Adding Subject and Certificate Store as tags
			tags := getSubjectTags(cert)
			tags = append(tags, "certificate_store:"+store)

			sender.Gauge("windows_certificate.days_remaining", daysRemaining, "", tags)

			if daysRemaining <= 0 {
				sender.ServiceCheck("windows_certificate.cert_expiration",
					servicecheck.ServiceCheckCritical,
					"",
					tags,
					"Certificate has expired")
			} else if daysRemaining < float64(w.config.DaysCritical) {
				sender.ServiceCheck("windows_certificate.cert_expiration",
					servicecheck.ServiceCheckCritical,
					"",
					tags,
					fmt.Sprintf("Certificate will expire in only %d days", int(daysRemaining)))
			} else if daysRemaining < float64(w.config.DaysWarning) {
				sender.ServiceCheck("windows_certificate.cert_expiration",
					servicecheck.ServiceCheckWarning,
					"",
					tags,
					fmt.Sprintf("Certificate wil expire in %d days", int(daysRemaining)))
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
	}

	return nil
}

func getCertificates(store string, certFilters []string) ([]*x509.Certificate, error) {
	certificates := []*x509.Certificate{}
	storeName := windows.StringToUTF16Ptr(store)

	log.Debugf("Opening certificate store: %s", store)
	storeHandle, err := windows.CertOpenStore(
		windows.CERT_STORE_PROV_SYSTEM,
		0,
		0,
		windows.CERT_SYSTEM_STORE_LOCAL_MACHINE|windows.CERT_STORE_READONLY_FLAG,
		uintptr(unsafe.Pointer(storeName)))
	if err != nil {
		log.Errorf("Error opening certificate store: %v", err)
		return nil, err
	}

	log.Debugf("Store handle: %v", storeHandle)

	// Close the store when we're done
	defer windows.CertCloseStore(storeHandle, 0)

	var certContext *windows.CertContext

	log.Debugf("Enumerating certificates in store")

	if len(certFilters) == 0 {
		certificates, err = getEnumCertificatesInStore(certContext, storeHandle)
	} else {
		certificates, err = findCertificatesInStore(certContext, storeHandle, certFilters)
	}
	if err != nil {
		log.Errorf("Error getting certificates: %v", err)
		return nil, err
	}

	log.Debugf("Found %d certificates in store", len(certificates))
	return certificates, nil
}

// getEnumCertificatesInStore retrieves all certificates in a certificate store
func getEnumCertificatesInStore(certContext *windows.CertContext, storeHandle windows.Handle) ([]*x509.Certificate, error) {
	var err error
	certificates := []*x509.Certificate{}
	for {
		certContext, err = windows.CertEnumCertificatesInStore(storeHandle, certContext)
		if err != nil {
			if certContext == nil {
				log.Debugf("No more certificates in store")
				break
			}
			log.Errorf("Error enumerating certificates in store: %v", err)
			return nil, err
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
func findCertificatesInStore(certContext *windows.CertContext, storeHandle windows.Handle, subjectFilters []string) ([]*x509.Certificate, error) {
	var err error
	certificates := []*x509.Certificate{}

	for _, subject := range subjectFilters {
		subjectName := windows.StringToUTF16Ptr(subject)

		for {
			certContext, err = windows.CertFindCertificateInStore(
				storeHandle,
				windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING,
				0,
				windows.CERT_FIND_SUBJECT_STR,
				unsafe.Pointer(subjectName),
				certContext,
			)
			if err != nil {
				if certContext == nil {
					log.Debugf("No more certificates in store")
					break
				}
				log.Errorf("Error enumerating certificates in store: %v", err)
				return nil, err
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
	case "0.9.2342.19200300.100.1.25":
		return "DC"
	default:
		return oid
	}
}
