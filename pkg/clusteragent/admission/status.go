// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"context"
	"embed"
	"io"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/comp/core/status"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	admprobe "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/probe"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var currentProbe atomic.Pointer[admprobe.Probe]

func setProbe(p *admprobe.Probe) {
	currentProbe.Store(p)
}

// getProbeStatus builds the probe status map for the agent status output.
// Always contains "Enabled"; also contains one of "Error", "Detail", or "HasStats"
// depending on state.
func getProbeStatus() map[string]interface{} {
	result := map[string]interface{}{}

	enabled := pkgconfigsetup.Datadog().GetBool("admission_controller.probe.enabled")
	result["Enabled"] = enabled
	if !enabled {
		return result
	}

	p := currentProbe.Load()
	if p == nil {
		result["Detail"] = "Waiting for the admission controller to start..."
		return result
	}

	if !p.IsLeader() {
		result["Detail"] = "The probe is only active on the leader instance."
		return result
	}

	stats := p.GetStatsForStatus()
	if ce, ok := stats["ConfigError"]; ok {
		result["Error"] = ce
		return result
	}

	result["HasStats"] = true
	for k, v := range stats {
		result[k] = v
	}
	return result
}

// GetStatus returns status info for the secret and webhook controllers.
func GetStatus(apiCl kubernetes.Interface) map[string]interface{} {
	status := make(map[string]interface{})
	if !pkgconfigsetup.Datadog().GetBool("admission_controller.enabled") {
		status["Disabled"] = "The admission controller is not enabled on the Cluster Agent"
		return status
	}

	ns := namespace.GetResourcesNamespace()
	webhookName := pkgconfigsetup.Datadog().GetString("admission_controller.webhook_name")
	secretName := pkgconfigsetup.Datadog().GetString("admission_controller.certificate.secret_name")
	status["WebhookName"] = webhookName
	status["SecretName"] = ns + "/" + secretName

	validatingWebhookStatus, validatingErr := admcommon.GetValidatingWebhookStatus(context.TODO(), webhookName, apiCl)
	if validatingErr != nil {
		status["ValidatingWebhookError"] = validatingErr.Error()
	} else {
		status["ValidatingWebhooks"] = validatingWebhookStatus
	}

	mutatingWebhookStatus, mutatingErr := admcommon.GetMutatingWebhookStatus(context.TODO(), webhookName, apiCl)
	if mutatingErr != nil {
		status["MutatingWebhookError"] = mutatingErr.Error()
	} else {
		status["MutatingWebhooks"] = mutatingWebhookStatus
	}

	secretStatus, secretErr := getSecretStatus(ns, secretName, apiCl)
	if secretErr != nil {
		status["SecretError"] = secretErr.Error()
	} else {
		status["Secret"] = secretStatus
	}

	// Running summarizes whether the enabled webhook types are registered with the API server.
	validationDown := validatingErr != nil && pkgconfigsetup.Datadog().GetBool("admission_controller.validation.enabled")
	mutationDown := mutatingErr != nil && pkgconfigsetup.Datadog().GetBool("admission_controller.mutation.enabled")
	status["Running"] = !validationDown && !mutationDown && secretErr == nil
	status["Probe"] = getProbeStatus()

	return status
}

func getSecretStatus(ns, name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	secretStatus := make(map[string]interface{})
	secret, err := apiCl.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return secretStatus, err
	}
	secretStatus["Name"] = secret.GetName()
	secretStatus["Namespace"] = secret.GetNamespace()
	secretStatus["CreatedAt"] = secret.GetCreationTimestamp()
	secretStatus["CABundleDigest"] = admcommon.Digest(secret.Data["cert.pem"])
	cert, err := certificate.GetCertFromSecret(secret.Data)
	if err != nil {
		log.Errorf("Cannot get certificate from secret: %v", err)
	}
	t := certificate.GetDurationBeforeExpiration(cert)
	secretStatus["CertValidDuration"] = t.String()
	return secretStatus, nil
}

// Provider provides the functionality to populate the status output
type Provider struct{}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (Provider) Name() string {
	return "Admission Controller"
}

// Section return the section
func (Provider) Section() string {
	return "Admission Controller"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(stats)

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "admissionwebhook.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func populateStatus(stats map[string]interface{}) {
	apiCl, apiErr := apiserver.GetAPIClient()
	if apiErr != nil {
		stats["admissionWebhook"] = map[string]string{"Error": apiErr.Error()}
	} else {
		stats["admissionWebhook"] = GetStatus(apiCl.Cl)
	}
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(stats)

	return stats
}
