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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	yy "github.com/ghodss/yaml"
	"github.com/swaggest/jsonschema-go"
	"github.com/xeipuuv/gojsonschema"
	yaml "go.yaml.in/yaml/v2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

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

	hcceLocalMachine = windows.Handle(1)

	// Certificate chain policy validation flags
	certChainPolicyIgnoreNotTimeValidFlag            = 0x00000001
	certChainPolicyIgnoreCtlNotTimeValidFlag         = 0x00000002
	certChainPolicyIgnoreNotTimeNestedFlag           = 0x00000004
	certChainPolicyIgnoreAllNotTimeValidFlags        = certChainPolicyIgnoreNotTimeValidFlag | certChainPolicyIgnoreCtlNotTimeValidFlag | certChainPolicyIgnoreNotTimeNestedFlag
	certChainPolicyIgnoreInvalidBasicConstraintsFlag = 0x00000008
	certChainPolicyAllowUnknownCaFlag                = 0x00000010
	certChainPolicyIgnoreWrongUsageFlag              = 0x00000020
	certChainPolicyIgnoreInvalidNameFlag             = 0x00000040
	certChainPolicyIgnoreInvalidPolicyFlag           = 0x00000080
	certChainPolicyIgnoreEndRevUnknownFlag           = 0x00000100
	certChainPolicyIgnoreCtlSignerRevUnknownFlag     = 0x00000200
	certChainPolicyIgnoreCaRevUnknownFlag            = 0x00000400
	certChainPolicyIgnoreRootRevUnknownFlag          = 0x00000800
	certChainPolicyIgnoreAllRevUnknownFlags          = certChainPolicyIgnoreEndRevUnknownFlag | certChainPolicyIgnoreCtlSignerRevUnknownFlag | certChainPolicyIgnoreCaRevUnknownFlag | certChainPolicyIgnoreRootRevUnknownFlag

	// CERT_HASH_PROP_ID is the property ID for the SHA-1 hash of the CRL
	//
	// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certgetcrlcontextproperty
	certHashPropID = 3
)

type certChainValidation struct {
	EnableCertChainValidation      bool     `yaml:"enabled" json:"enabled" default:"false"`
	CertChainPolicyValidationFlags []string `yaml:"policy_validation_flags" json:"policy_validation_flags" nullable:"true"`
}

// Config is the configuration options for this check
// it is exported so that the yaml parser can read it.
type Config struct {
	CertificateStore    string              `yaml:"certificate_store" json:"certificate_store" required:"true" nullable:"false"`
	CertSubjects        []string            `yaml:"certificate_subjects" json:"certificate_subjects" nullable:"false"`
	Server              string              `yaml:"server" json:"server" nullable:"false"`
	Username            string              `yaml:"username" json:"username" nullable:"false"`
	Password            string              `yaml:"password" json:"password" nullable:"false"`
	DaysCritical        int                 `yaml:"days_critical" json:"days_critical" minimum:"0"`
	DaysWarning         int                 `yaml:"days_warning" json:"days_warning" minimum:"0"`
	EnableCRLMonitoring bool                `yaml:"enable_crl_monitoring" json:"enable_crl_monitoring" default:"false"`
	CrlDaysWarning      int                 `yaml:"crl_days_warning" json:"crl_days_warning" minimum:"0"`
	CertChainValidation certChainValidation `yaml:"cert_chain_validation" json:"cert_chain_validation" nullable:"true"`
}

// WinCertChk is the object representing the check
type WinCertChk struct {
	core.CheckBase
	config Config
}

type crlInfoCopy struct {
	Issuer     string
	NextUpdate time.Time
	Thumbprint string
}

type certInfo struct {
	Certificate      *x509.Certificate
	TrustStatusError uint32 // windows.TrustStatus.ErrorStatus
	ChainPolicyError uint32 // windows.CertChainPolicyStatus.Error
	Thumbprint       string
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
		return fmt.Errorf("failed to create config validation schema: %s", err)
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
		return errors.New("configuration validation failed")
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

	var certificates []certInfo
	var crlInfo []crlInfoCopy
	var serverTag string
	if w.config.Server != "" {
		certificates, crlInfo, err = w.getRemoteCertificates(w.config.CertificateStore, w.config.CertSubjects, w.config.Server, w.config.Username, w.config.Password, w.config.EnableCRLMonitoring)
		if err != nil {
			return err
		}
		serverTag = "server:" + w.config.Server
	} else {
		certificates, crlInfo, err = w.getCertificates(w.config.CertificateStore, w.config.CertSubjects, w.config.EnableCRLMonitoring)
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
		log.Debugf("Found certificate: %s", cert.Certificate.Subject.String())
		daysRemaining := getExpiration(cert.Certificate.NotAfter)
		expirationDate := cert.Certificate.NotAfter.Format(time.RFC3339)

		// Adding Subject and Certificate Store as tags
		tags := getSubjectTags(cert.Certificate)
		tags = append(tags, "certificate_store:"+w.config.CertificateStore)
		tags = append(tags, serverTag)
		tags = append(tags, "certificate_thumbprint:"+cert.Thumbprint)
		// Need to use hex format for serial numbers as they are typically displayed in hex format in the UI
		tags = append(tags, "certificate_serial_number:"+cert.Certificate.SerialNumber.Text(16))
		sender.Gauge("windows_certificate.days_remaining", daysRemaining, "", tags)

		if daysRemaining <= 0 {
			sender.ServiceCheck("windows_certificate.cert_expiration",
				servicecheck.ServiceCheckCritical,
				"",
				tags,
				"Certificate has expired. Certificate expiration date is "+expirationDate)
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

		if w.config.CertChainValidation.EnableCertChainValidation {
			// Report both the trust status and chain policy errors if they exist
			if cert.TrustStatusError != 0 {
				log.Debugf("Certificate %s has trust status error: %d", cert.Certificate.Subject.String(), cert.TrustStatusError)
				trustStatusErrors := getCertChainTrustStatusErrors(cert.TrustStatusError)
				message := "Certificate Validation failed. The certificates in the certificate chain have the following errors: " + strings.Join(trustStatusErrors, ", ")
				if cert.ChainPolicyError != 0 {
					chainPolicyError := getCertChainPolicyErrors(cert.ChainPolicyError)
					message = message + ", " + chainPolicyError
				}
				sender.ServiceCheck("windows_certificate.cert_chain_validation",
					servicecheck.ServiceCheckCritical,
					"",
					tags,
					message,
				)
				// Report the chain policy error only if it exists
			} else if cert.ChainPolicyError != 0 {
				log.Debugf("Certificate %s has chain policy error: %d", cert.Certificate.Subject.String(), cert.ChainPolicyError)
				chainPolicyError := getCertChainPolicyErrors(cert.ChainPolicyError)
				sender.ServiceCheck("windows_certificate.cert_chain_validation",
					servicecheck.ServiceCheckCritical,
					"",
					tags,
					chainPolicyError,
				)
				// Report OK if there are no errors
			} else {
				sender.ServiceCheck("windows_certificate.cert_chain_validation",
					servicecheck.ServiceCheckOK,
					"",
					tags,
					"",
				)
			}
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
		crlTags = append(crlTags, "crl_thumbprint:"+crl.Thumbprint)
		sender.Gauge("windows_certificate.crl_days_remaining", crlDaysRemaining, "", crlTags)

		if crlDaysRemaining <= 0 {
			sender.ServiceCheck("windows_certificate.crl_expiration",
				servicecheck.ServiceCheckCritical,
				"",
				crlTags,
				"CRL has expired. CRL expiration date is "+crlExpirationDate)
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

func (w *WinCertChk) getCertificates(store string, certFilters []string, collectCRL bool) ([]certInfo, []crlInfoCopy, error) {
	var certificates []certInfo
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
		certificates, err = getEnumCertificatesInStore(storeHandle, w.config.CertChainValidation)
	} else {
		certificates, err = findCertificatesInStore(storeHandle, certFilters, w.config.CertChainValidation)
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

func (w *WinCertChk) getRemoteCertificates(store string, certFilters []string, server string, username string, password string, collectCRL bool) ([]certInfo, []crlInfoCopy, error) {

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
	var certificates []certInfo

	if len(certFilters) == 0 {
		certificates, err = getEnumCertificatesInStore(storeHandle, w.config.CertChainValidation)
	} else {
		certificates, err = findCertificatesInStore(storeHandle, certFilters, w.config.CertChainValidation)
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
func getEnumCertificatesInStore(storeHandle windows.Handle, certChainValidation certChainValidation) ([]certInfo, error) {
	var err error
	certificates := []certInfo{}

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

		var trustStatusError uint32
		var chainPolicyError uint32
		if certChainValidation.EnableCertChainValidation {
			trustStatusError, chainPolicyError, err = validateCertificateChain(certContext, storeHandle, certChainValidation.CertChainPolicyValidationFlags)
			if err != nil {
				log.Errorf("Error validating certificate chain: %v", err)
				continue
			}
		}

		certThumbprint, err := getCertThumbprint(certContext)
		if err != nil {
			log.Errorf("Error getting certificate thumbprint: %v", err)
			continue
		}

		certificates = append(certificates, certInfo{
			Certificate:      cert,
			TrustStatusError: trustStatusError,
			ChainPolicyError: chainPolicyError,
			Thumbprint:       certThumbprint,
		})
	}

	return certificates, nil
}

// findCertificatesInStore finds certificates in a store with a given subject string
func findCertificatesInStore(storeHandle windows.Handle, subjectFilters []string, certChainValidation certChainValidation) ([]certInfo, error) {
	var err error
	certificates := []certInfo{}

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

			var trustStatusError uint32
			var chainPolicyError uint32
			if certChainValidation.EnableCertChainValidation {
				trustStatusError, chainPolicyError, err = validateCertificateChain(certContext, storeHandle, certChainValidation.CertChainPolicyValidationFlags)
				if err != nil {
					log.Errorf("Error validating certificate chain: %v", err)
					continue
				}
			}

			certThumbprint, err := getCertThumbprint(certContext)
			if err != nil {
				log.Errorf("Error getting certificate thumbprint: %v", err)
				continue
			}

			certificates = append(certificates, certInfo{
				Certificate:      cert,
				TrustStatusError: trustStatusError,
				ChainPolicyError: chainPolicyError,
				Thumbprint:       certThumbprint,
			})
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

		crlThumbprint, err := getCrlThumbprint(crlContext)
		if err != nil {
			log.Errorf("Error getting CRL thumbprint: %v", err)
			continue
		}

		crl := crlInfoCopy{
			Issuer:     issuerStr,
			NextUpdate: time.Unix(0, pCrlInfo.NextUpdate.Nanoseconds()),
			Thumbprint: crlThumbprint,
		}

		crlInfo = append(crlInfo, crl)
	}

	return crlInfo, nil
}

func validateCertificateChain(certContext *windows.CertContext, storeHandle windows.Handle, ignoreFlags []string) (uint32, uint32, error) {
	var trustStatusError uint32
	var chainPolicyError uint32
	var chainPara windows.CertChainPara
	chainPara.Size = uint32(unsafe.Sizeof(chainPara))

	var pChainContext *windows.CertChainContext
	defer func() {
		if pChainContext != nil {
			log.Debugf("Freeing certificate chain")
			windows.CertFreeCertificateChain(pChainContext)
		}
	}()
	log.Debugf("Getting certificate chain")
	err := windows.CertGetCertificateChain(
		hcceLocalMachine, // hChainEngine (use local machine engine)
		certContext,      // pCertContext
		nil,              // pTime (use current time)
		storeHandle,      // hAdditionalStore
		&chainPara,       // pChainPara
		0,                // dwFlags
		0,                // pvReserved
		&pChainContext,   // ppChainContext
	)
	if err != nil {
		log.Errorf("Error getting certificate chain: %v", err)
		return 0, 0, err
	}
	log.Debugf("Certificate chain retrieved successfully")
	trustStatusError = pChainContext.TrustStatus.ErrorStatus

	var pPolicyPara windows.CertChainPolicyPara
	pPolicyPara.Size = uint32(unsafe.Sizeof(pPolicyPara))
	var pPolicyStatus windows.CertChainPolicyStatus
	pPolicyStatus.Size = uint32(unsafe.Sizeof(pPolicyStatus))
	pPolicyPara.Flags = setCertChainValidationFlags(ignoreFlags)

	log.Debugf("Verifying certificate chain policy")
	err = windows.CertVerifyCertificateChainPolicy(
		windows.CERT_CHAIN_POLICY_BASE,
		pChainContext,
		&pPolicyPara,
		&pPolicyStatus,
	)
	if err != nil {
		log.Errorf("Error verifying certificate chain policy: %v", err)
		return 0, 0, err
	}
	log.Debugf("Certificate chain policy verified successfully")
	chainPolicyError = pPolicyStatus.Error

	return trustStatusError, chainPolicyError, nil
}

func setCertChainValidationFlags(ignoreFlags []string) uint32 {
	var flags uint32
	for _, flag := range ignoreFlags {
		flags |= getCertChainFlagFromString(flag)
	}
	return flags
}

func getCertChainFlagFromString(flag string) uint32 {
	switch flag {
	case "CERT_CHAIN_POLICY_IGNORE_NOT_TIME_VALID_FLAG":
		return certChainPolicyIgnoreNotTimeValidFlag
	case "CERT_CHAIN_POLICY_IGNORE_CTL_NOT_TIME_VALID_FLAG":
		return certChainPolicyIgnoreCtlNotTimeValidFlag
	case "CERT_CHAIN_POLICY_IGNORE_NOT_TIME_NESTED_FLAG":
		return certChainPolicyIgnoreNotTimeNestedFlag
	case "CERT_CHAIN_POLICY_IGNORE_ALL_NOT_TIME_VALID_FLAGS":
		return certChainPolicyIgnoreAllNotTimeValidFlags
	case "CERT_CHAIN_POLICY_IGNORE_INVALID_BASIC_CONSTRAINTS_FLAG":
		return certChainPolicyIgnoreInvalidBasicConstraintsFlag
	case "CERT_CHAIN_POLICY_ALLOW_UNKNOWN_CA_FLAG":
		return certChainPolicyAllowUnknownCaFlag
	case "CERT_CHAIN_POLICY_IGNORE_WRONG_USAGE_FLAG":
		return certChainPolicyIgnoreWrongUsageFlag
	case "CERT_CHAIN_POLICY_IGNORE_INVALID_NAME_FLAG":
		return certChainPolicyIgnoreInvalidNameFlag
	case "CERT_CHAIN_POLICY_IGNORE_INVALID_POLICY_FLAG":
		return certChainPolicyIgnoreInvalidPolicyFlag
	case "CERT_CHAIN_POLICY_IGNORE_END_REV_UNKNOWN_FLAG":
		return certChainPolicyIgnoreEndRevUnknownFlag
	case "CERT_CHAIN_POLICY_IGNORE_CTL_SIGNER_REV_UNKNOWN_FLAG":
		return certChainPolicyIgnoreCtlSignerRevUnknownFlag
	case "CERT_CHAIN_POLICY_IGNORE_CA_REV_UNKNOWN_FLAG":
		return certChainPolicyIgnoreCaRevUnknownFlag
	case "CERT_CHAIN_POLICY_IGNORE_ROOT_REV_UNKNOWN_FLAG":
		return certChainPolicyIgnoreRootRevUnknownFlag
	case "CERT_CHAIN_POLICY_IGNORE_ALL_REV_UNKNOWN_FLAGS":
		return certChainPolicyIgnoreAllRevUnknownFlags
	default:
		log.Warnf("Unknown certificate chain validation flag, %s. Flag will be ignored.", flag)
		return 0
	}
}

func getCertChainTrustStatusErrors(trustStatusError uint32) []string {
	var errors []string
	if trustStatusError&windows.CERT_TRUST_IS_NOT_TIME_VALID != 0 {
		errors = append(errors, "Not time valid")
	}
	if trustStatusError&windows.CERT_TRUST_IS_REVOKED != 0 {
		errors = append(errors, "Revoked")
	}
	if trustStatusError&windows.CERT_TRUST_IS_NOT_SIGNATURE_VALID != 0 {
		errors = append(errors, "Not signature valid")
	}
	if trustStatusError&windows.CERT_TRUST_IS_NOT_VALID_FOR_USAGE != 0 {
		errors = append(errors, "Not valid for usage")
	}
	if trustStatusError&windows.CERT_TRUST_IS_UNTRUSTED_ROOT != 0 {
		errors = append(errors, "Untrusted root")
	}
	if errors == nil {
		errors = append(errors, "Unknown error")
	}
	return errors
}

func getCertChainPolicyErrors(chainPolicyError uint32) string {
	errorMessage := fmt.Sprintf("Certificate chain policy validation failed with the following error 0x%X: %v", chainPolicyError, windows.Errno(chainPolicyError))
	return errorMessage
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
