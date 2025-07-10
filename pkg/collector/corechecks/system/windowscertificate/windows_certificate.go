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
	"os"
	"strings"
	"time"
	"unsafe"

	yy "github.com/ghodss/yaml"
	"github.com/swaggest/jsonschema-go"
	"github.com/xeipuuv/gojsonschema"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// CheckName is the name of the check
	CheckName = "windows_certificate"

	defaultMinCollectionInterval = 300
	defaultDaysCritical          = 7
	defaultDaysWarning           = 14
	defaultCrlDaysWarning        = 0

	certStoreOpenFlags = windows.CERT_SYSTEM_STORE_LOCAL_MACHINE | windows.CERT_STORE_READONLY_FLAG | windows.CERT_STORE_OPEN_EXISTING_FLAG
	certEncoding       = windows.X509_ASN_ENCODING | windows.PKCS_7_ASN_ENCODING
	cryptENotFound     = windows.Errno(windows.CRYPT_E_NOT_FOUND)
	eInvalidArg        = windows.Errno(windows.E_INVALIDARG)

	// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certnametostrw
	//
	// CERT_X500_NAME_STR converts OIDs to their X.500 key names
	certX500NameStr = 3

	// CERT_NAME_STR_CRLF_FLAG replaces commas with a \r\n separator
	certNameStrCRLF = 0x08000000
)

// Config is the configuration options for this check
// it is exported so that the yaml parser can read it.
type Config struct {
	CertificateStore    string   `yaml:"certificate_store" json:"certificate_store" required:"true" nullable:"false"`
	CertSubjects        []string `yaml:"certificate_subjects" json:"certificate_subjects" nullable:"false"`
	Server              string   `yaml:"server" json:"server" nullable:"false"`
	Username            string   `yaml:"username" json:"username" nullable:"false"`
	Password            string   `yaml:"password" json:"password" nullable:"false"`
	DaysCritical        int      `yaml:"days_critical" json:"days_critical" minimum:"0"`
	DaysWarning         int      `yaml:"days_warning" json:"days_warning" minimum:"0"`
	EnableCRLMonitoring bool     `yaml:"enable_crl_monitoring" json:"enable_crl_monitoring" default:"false"`
	CrlDaysWarning      int      `yaml:"crl_days_warning" json:"crl_days_warning" minimum:"0"`
}

// WinCertChk is the object representing the check
type WinCertChk struct {
	core.CheckBase
	config Config
}

type crlInfoCopy struct {
	Issuer     string
	NextUpdate time.Time
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
		DaysCritical:   defaultDaysCritical,
		DaysWarning:    defaultDaysWarning,
		CrlDaysWarning: defaultCrlDaysWarning,
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

	var certificates []*x509.Certificate
	var crlInfo []crlInfoCopy
	var serverTag string
	if w.config.Server != "" {
		certificates, crlInfo, err = getRemoteCertificates(w.config.CertificateStore, w.config.CertSubjects, w.config.Server, w.config.Username, w.config.Password, w.config.EnableCRLMonitoring)
		if err != nil {
			return err
		}
		serverTag = "server:" + w.config.Server
	} else {
		certificates, crlInfo, err = getCertificates(w.config.CertificateStore, w.config.CertSubjects, w.config.EnableCRLMonitoring)
		if err != nil {
			return err
		}
		hostname, err := os.Hostname()
		if err != nil {
			return err
		}
		serverTag = "server:" + hostname
	}
	if len(certificates) == 0 {
		log.Warnf("No certificates found in store: %s for subject filters: '%s'", w.config.CertificateStore, strings.Join(w.config.CertSubjects, ", "))
	}
	if len(crlInfo) == 0 && w.config.EnableCRLMonitoring {
		log.Warnf("No CRLs found in store: %s", w.config.CertificateStore)
	}

	for _, cert := range certificates {
		log.Debugf("Found certificate: %s", cert.Subject.String())
		daysRemaining := getExpiration(cert.NotAfter)
		expirationDate := cert.NotAfter.Format(time.RFC3339)

		// Adding Subject and Certificate Store as tags
		tags := getSubjectTags(cert)
		tags = append(tags, "certificate_store:"+w.config.CertificateStore)
		tags = append(tags, serverTag)
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

	for _, crl := range crlInfo {
		crlIssuer := crl.Issuer
		log.Debugf("Found CRL Issued by: %s", crlIssuer)

		crlDaysRemaining := getExpiration(crl.NextUpdate)
		crlExpirationDate := crl.NextUpdate.Format(time.RFC3339)

		// Adding CRL Issuer and Certificate Store as tags
		crlTags := getCrlIssuerTags(crlIssuer)
		crlTags = append(crlTags, "certificate_store:"+w.config.CertificateStore)
		crlTags = append(crlTags, serverTag)
		sender.Gauge("windows_certificate.crl_days_remaining", crlDaysRemaining, "", crlTags)

		if crlDaysRemaining <= 0 {
			sender.ServiceCheck("windows_certificate.crl_expiration",
				servicecheck.ServiceCheckCritical,
				"",
				crlTags,
				fmt.Sprintf("CRL has expired. CRL expiration date is %s", crlExpirationDate))
		} else if crlDaysRemaining < float64(w.config.CrlDaysWarning) {
			sender.ServiceCheck("windows_certificate.crl_expiration",
				servicecheck.ServiceCheckWarning,
				"",
				crlTags,
				fmt.Sprintf("CRL will expire in %.2f days. CRL expiration date is %s", crlDaysRemaining, crlExpirationDate))
		} else {
			sender.ServiceCheck(
				"windows_certificate.crl_expiration",
				servicecheck.ServiceCheckOK,
				"",
				crlTags,
				"",
			)
		}

	}

	return nil
}

func getCertificates(store string, certFilters []string, collectCRL bool) ([]*x509.Certificate, []crlInfoCopy, error) {
	var certificates []*x509.Certificate
	var crlInfo []crlInfoCopy
	storeName := windows.StringToUTF16Ptr(store)

	log.Debugf("Opening certificate store: %s", store)
	storeHandle, err := openCertificateStore(
		windows.CERT_STORE_PROV_SYSTEM,
		certStoreOpenFlags,
		uintptr(unsafe.Pointer(storeName)))
	if err != nil {
		log.Errorf("Error opening certificate store %s: %v", store, err)
		return nil, nil, err
	}
	// Close the store when the function returns
	defer closeCertificateStore(storeHandle, store)

	log.Debugf("Enumerating certificates in store")

	if len(certFilters) == 0 {
		certificates, err = getEnumCertificatesInStore(storeHandle)
	} else {
		certificates, err = findCertificatesInStore(storeHandle, certFilters)
	}
	if err != nil {
		log.Errorf("Error getting certificates: %v", err)
		return nil, nil, err
	}
	log.Debugf("Found %d certificates in store %s", len(certificates), store)

	if collectCRL {
		crlInfo, err = getCrlInfo(storeHandle)
		if err != nil {
			log.Errorf("Error getting CRLs: %v", err)
			return nil, nil, err
		}
	}
	log.Debugf("Found %d CRLs in store %s", len(crlInfo), store)

	return certificates, crlInfo, nil
}

func getRemoteCertificates(store string, certFilters []string, server string, username string, password string, collectCRL bool) ([]*x509.Certificate, []crlInfoCopy, error) {

	// Create network path to the remote server's IPC$ share
	// see https://learn.microsoft.com/en-us/windows/win32/api/winreg/nf-winreg-regconnectregistryw
	remoteServer := "\\\\" + server + "\\IPC$"
	registryPath := "SOFTWARE\\Microsoft\\SystemCertificates\\" + store
	var remoteRegKey registry.Key
	var certStoreKey registry.Key

	err := netAddConnection(remoteServer, "", password, username)
	if err != nil {
		log.Errorf("Error adding connection: %v", err)
		return nil, nil, err
	}
	log.Debugf("Connection to %s is successful", server)
	defer func() {
		err = netCancelConnection(remoteServer)
		if err != nil {
			log.Errorf("Error canceling connection: %v", err)
		}
	}()

	log.Debugf("Opening remote registry on %s", server)

	// After establishing a connection to the remote server, we open its Local Machine registry key
	remoteRegKey, err = registry.OpenRemoteKey(server, registry.LOCAL_MACHINE)
	if err != nil {
		log.Errorf("Error opening remote registry key for server %s: %v For more information see, https://learn.microsoft.com/en-us/windows/win32/api/winreg/nf-winreg-regconnectregistryw#remarks", server, err)
		return nil, nil, err
	}
	log.Debugf("Remote registry opened successfully")
	defer remoteRegKey.Close()

	// Once the remote registry is opened, we use its handle to open the registry key of the certificate store
	certStoreKey, err = registry.OpenKey(remoteRegKey, registryPath, registry.READ)
	if err != nil {
		log.Errorf("Error opening %s registry key for server %s: %v For more information see, https://learn.microsoft.com/en-us/windows/win32/api/winreg/nf-winreg-regopenkeyexw", registryPath, server, err)
		return nil, nil, err
	}
	log.Debugf("%s registry key opened successfully", registryPath)
	defer certStoreKey.Close()

	// Pass the registry key handle of the certificate store with the windows.CERT_STORE_PROV_REG provider
	storeHandle, err := openCertificateStore(
		windows.CERT_STORE_PROV_REG,
		windows.CERT_STORE_OPEN_EXISTING_FLAG,
		uintptr(certStoreKey))
	if err != nil {
		log.Errorf("Error opening certificate store %s: %v", store, err)
		return nil, nil, err
	}
	log.Debugf("Certificate store opened successfully")
	defer closeCertificateStore(storeHandle, store)

	log.Debugf("Enumerating certificates in store")
	var certificates []*x509.Certificate

	if len(certFilters) == 0 {
		certificates, err = getEnumCertificatesInStore(storeHandle)
	} else {
		certificates, err = findCertificatesInStore(storeHandle, certFilters)
	}
	if err != nil {
		log.Errorf("Error getting certificates: %v", err)
		return nil, nil, err
	}
	log.Debugf("Found %d certificates in store %s", len(certificates), store)

	var crlInfo []crlInfoCopy
	if collectCRL {
		crlInfo, err = getCrlInfo(storeHandle)
		if err != nil {
			log.Errorf("Error getting CRLs: %v", err)
			return nil, nil, err
		}
	}
	log.Debugf("Found %d CRLs in store %s", len(crlInfo), store)
	return certificates, crlInfo, nil
}

func openCertificateStore(storeProvider uintptr, flags uint32, para uintptr) (windows.Handle, error) {
	storeHandle, err := windows.CertOpenStore(
		storeProvider,
		0,
		0,
		flags,
		para,
	)
	if err != nil {
		return 0, err
	}
	return storeHandle, nil
}

func closeCertificateStore(storeHandle windows.Handle, store string) {
	err := windows.CertCloseStore(storeHandle, 0)
	if err != nil {
		log.Errorf("Error closing certificate store %s: %v", store, err)
	}
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

func getCrlInfo(storeHandle windows.Handle) ([]crlInfoCopy, error) {
	var err error
	var crlContext *winutil.CRLContext
	var crlInfo []crlInfoCopy
	defer func() {
		if crlContext != nil {
			log.Debugf("Freeing CRL context")
			err = winutil.CertFreeCRLContext(crlContext)
			if err != nil {
				log.Errorf("Error freeing CRL context: %v", err)
			}
		}
	}()

	for {
		crlContext, err = winutil.CertEnumCRLsInStore(storeHandle, crlContext)
		if err == cryptENotFound {
			log.Debugf("No matching CRLs found: %v", err)
			break
		} else if err != nil {
			log.Errorf("Error enumerating CRL: %v", err)
			return nil, err
		}

		if crlContext.PCrlInfo == nil {
			log.Errorf("CRL info pointer is nil")
			continue
		}

		pCrlInfo := (*winutil.CRLInfo)(unsafe.Pointer(crlContext.PCrlInfo))
		issuerStr, err := convertCertNameBlobToString(&pCrlInfo.Issuer)
		if err != nil {
			log.Errorf("Error converting CRL issuer to string: %v", err)
			continue
		}
		crl := crlInfoCopy{
			Issuer:     issuerStr,
			NextUpdate: time.Unix(0, pCrlInfo.NextUpdate.Nanoseconds()),
		}

		crlInfo = append(crlInfo, crl)
	}

	return crlInfo, nil
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

func getExpiration(expirationDate time.Time) float64 {
	daysRemaining := time.Until(expirationDate).Hours() / 24
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

// getCrlIssuerTags returns the tags for the CRL issuer
// For example: [L=Internet, O="VeriSign, Inc.", OU=VeriSign Commercial Software Publishers CA]
// will become: [crl_issuer_L:Internet, crl_issuer_O:VeriSign, Inc., crl_issuer_OU:VeriSign Commercial Software Publishers CA]
func getCrlIssuerTags(issuer string) []string {
	issuerTags := []string{}

	if issuer == "" {
		return issuerTags
	}

	issuerRDNs := strings.Split(issuer, "\r\n")
	for _, rdn := range issuerRDNs {
		rdn = strings.TrimSpace(rdn)
		if rdn == "" {
			continue
		}

		// Split by "=" to get key and value
		keyValue := strings.SplitN(rdn, "=", 2)
		if len(keyValue) == 2 {
			key := strings.TrimSpace(keyValue[0])
			value := strings.TrimSpace(keyValue[1])

			// Remove quotes if present
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = strings.Trim(value, "\"")
			}

			// Format: "crl_issuer_<key>:<value>"
			component := fmt.Sprintf("crl_issuer_%s:%s", key, value)
			issuerTags = append(issuerTags, component)
		}
	}
	log.Debugf("CRL issuer tags: %v", issuerTags)

	return issuerTags
}

func convertCertNameBlobToString(nameBlob *windows.CertNameBlob) (string, error) {
	if nameBlob == nil || nameBlob.Size == 0 {
		return "", nil
	}
	dwStrType := uint32(certX500NameStr | certNameStrCRLF)

	var strBuffer []uint16
	_, bufferSize, err := winutil.CertNameToStrW(windows.X509_ASN_ENCODING, nameBlob, dwStrType, nil, 0)

	if err != nil {
		return "", err
	}

	strBuffer = make([]uint16, bufferSize)
	certNameStr, _, err := winutil.CertNameToStrW(windows.X509_ASN_ENCODING, nameBlob, dwStrType, strBuffer, bufferSize)
	if err != nil {
		return "", err
	}
	return certNameStr, nil
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

func netAddConnection(remoteName, localName, password, username string) error {
	netResource, err := winutil.CreateNetResource(remoteName, localName, "", "", 0, 0, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to create NetResource: %v", err)
	}
	log.Debugf(
		"Created NetResource for connection to %s: {Scope: %d, Type: %d, DisplayType: %d, Usage: %d, localName: %s, remoteName: %s, comment: %s, provider: %s}",
		remoteName,
		netResource.Scope,
		netResource.Type,
		netResource.DisplayType,
		netResource.Usage,
		windows.UTF16PtrToString(netResource.LocalName),
		windows.UTF16PtrToString(netResource.RemoteName),
		windows.UTF16PtrToString(netResource.Comment),
		windows.UTF16PtrToString(netResource.Provider),
	)
	return winutil.WNetAddConnection2(&netResource, password, username, 0)
}

func netCancelConnection(name string) error {
	log.Debugf("Canceling connection to %s", name)
	return winutil.WNetCancelConnection2(name)
}
