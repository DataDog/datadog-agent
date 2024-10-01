// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

// Package admission runs the admission controller managed by the Cluster Agent.
package admission

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	stdLog "log"
	"net/http"
	"time"

	"github.com/cihub/seelog"
	admiv1 "k8s.io/api/admission/v1"
	admiv1beta1 "k8s.io/api/admission/v1beta1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	admicommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

const jsonContentType = "application/json"

// Request contains the information of an admission request
type Request struct {
	// UID is the unique identifier of the AdmissionRequest
	UID types.UID
	// Name is the name of the object
	Name string
	// Namespace is the namespace of the object
	Namespace string
	// Kind is the kind of the object
	Kind metav1.GroupVersionKind
	// Resource is the resource of the object
	Resource metav1.GroupVersionResource
	// Operation is the operation of the request
	Operation admissionregistrationv1.OperationType
	// UserInfo contains information about the requesting user
	UserInfo *authenticationv1.UserInfo
	// Object is the new object being admitted. It is null for DELETE operations
	Object []byte
	// OldObject is the existing object. It is null for CREATE and CONNECT operations
	OldObject []byte
	// DynamicClient holds a dynamic Kubernetes client
	DynamicClient dynamic.Interface
	// APIClient holds a Kubernetes client
	APIClient kubernetes.Interface
}

// WebhookFunc is the function that runs the webhook logic.
// We always return an `admissionv1.AdmissionResponse` as it will be converted to the appropriate version if needed.
type WebhookFunc func(request *Request) *admiv1.AdmissionResponse

// Server TODO <container-integrations>
type Server struct {
	decoder runtime.Decoder
	mux     *http.ServeMux
}

// NewServer creates an admission webhook server.
func NewServer() *Server {
	s := &Server{
		mux: http.NewServeMux(),
	}

	s.initDecoder()

	return s
}

// initDecoder sets the server's decoder.
func (s *Server) initDecoder() {
	scheme := runtime.NewScheme()
	err := admiv1.AddToScheme(scheme)
	if err != nil {
		log.Warnf("Couldn't register the admission/v1 scheme: %v", err)
	}

	err = admiv1beta1.AddToScheme(scheme)
	if err != nil {
		log.Warnf("Couldn't register the admission/v1beta1 scheme: %v", err)
	}

	s.decoder = serializer.NewCodecFactory(scheme).UniversalDeserializer()
}

// Register adds an admission webhook handler.
// Register must be called to register the desired webhook handlers before calling Run.
func (s *Server) Register(uri string, webhookName string, webhookType admicommon.WebhookType, f WebhookFunc, dc dynamic.Interface, apiClient kubernetes.Interface) {
	s.mux.HandleFunc(uri, func(w http.ResponseWriter, r *http.Request) {
		s.handle(w, r, webhookName, webhookType, f, dc, apiClient)
	})
}

// Run starts the kubernetes admission webhook server.
func (s *Server) Run(mainCtx context.Context, client kubernetes.Interface) error {
	var tlsMinVersion uint16 = tls.VersionTLS13
	if pkgconfigsetup.Datadog().GetBool("cluster_agent.allow_legacy_tls") {
		tlsMinVersion = tls.VersionTLS10
	}

	logWriter, _ := pkglogsetup.NewTLSHandshakeErrorWriter(4, seelog.WarnLvl)
	server := &http.Server{
		Addr:     fmt.Sprintf(":%d", pkgconfigsetup.Datadog().GetInt("admission_controller.port")),
		Handler:  s.mux,
		ErrorLog: stdLog.New(logWriter, "Error from the admission controller http API server: ", 0),
		TLSConfig: &tls.Config{
			GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
				secretNs := common.GetResourcesNamespace()
				secretName := pkgconfigsetup.Datadog().GetString("admission_controller.certificate.secret_name")
				cert, err := certificate.GetCertificateFromSecret(secretNs, secretName, client)
				if err != nil {
					log.Errorf("Couldn't fetch certificate: %v", err)
				}
				return cert, nil
			},
			MinVersion: tlsMinVersion,
		},
	}
	go func() error {
		return log.Error(server.ListenAndServeTLS("", ""))
	}() //nolint:errcheck

	<-mainCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// handle contains the main logic responsible for handling admission requests.
// It supports both v1 and v1beta1 requests.
func (s *Server) handle(w http.ResponseWriter, r *http.Request, webhookName string, webhookType admicommon.WebhookType, webhookFunc WebhookFunc, dc dynamic.Interface, apiClient kubernetes.Interface) {
	// Increment the metrics for the received webhook.
	// We send the webhook name twice to keep the backward compatibility with `mutation_type` tag.
	metrics.WebhooksReceived.Inc(webhookName, webhookName, webhookType.String())

	// Measure the time it takes to process the request.
	start := time.Now()
	defer func() {
		// We send the webhook name twice to keep the backward compatibility with `mutation_type` tag.
		metrics.WebhooksResponseDuration.Observe(time.Since(start).Seconds(), webhookName, webhookName, webhookType.String())
	}()

	// Validate admission request.
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		log.Warnf("Invalid method %s, only POST requests are allowed", r.Method)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Warnf("Could not read request body: %v", err)
		return
	}
	defer r.Body.Close()

	if contentType := r.Header.Get("Content-Type"); contentType != jsonContentType {
		w.WriteHeader(http.StatusBadRequest)
		log.Warnf("Unsupported content type %s, only %s is supported", contentType, jsonContentType)
		return
	}

	// Deserialize admission request.
	obj, gvk, err := s.decoder.Decode(body, nil, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Warnf("Could not deserialize request: %v", err)
		return
	}

	// Handle the request based on `GroupVersionKind`.
	var response runtime.Object
	switch *gvk {
	case admiv1.SchemeGroupVersion.WithKind("AdmissionReview"):
		admissionReviewReq, ok := obj.(*admiv1.AdmissionReview)
		if !ok {
			log.Errorf("Expected v1.AdmissionReview, got type %T", obj)
		}

		admissionReview := &admiv1.AdmissionReview{}
		admissionReview.SetGroupVersionKind(*gvk)
		admissionRequest := Request{
			UID:           admissionReviewReq.Request.UID,
			Kind:          admissionReviewReq.Request.Kind,
			Resource:      admissionReviewReq.Request.Resource,
			Name:          admissionReviewReq.Request.Name,
			Namespace:     admissionReviewReq.Request.Namespace,
			Operation:     admissionregistrationv1.OperationType(admissionReviewReq.Request.Operation),
			UserInfo:      &admissionReviewReq.Request.UserInfo,
			Object:        admissionReviewReq.Request.Object.Raw,
			OldObject:     admissionReviewReq.Request.OldObject.Raw,
			DynamicClient: dc,
			APIClient:     apiClient,
		}

		// Generate admission response
		admissionResponse := webhookFunc(&admissionRequest)
		admissionReview.Response = admissionResponse
		admissionReview.Response.UID = admissionReviewReq.Request.UID
		response = admissionReview
	case admiv1beta1.SchemeGroupVersion.WithKind("AdmissionReview"):
		admissionReviewReq, ok := obj.(*admiv1beta1.AdmissionReview)
		if !ok {
			log.Errorf("Expected v1beta1.AdmissionReview, got type %T", obj)
		}

		admissionReview := &admiv1beta1.AdmissionReview{}
		admissionReview.SetGroupVersionKind(*gvk)
		admissionRequest := Request{
			UID:           admissionReviewReq.Request.UID,
			Kind:          admissionReviewReq.Request.Kind,
			Resource:      admissionReviewReq.Request.Resource,
			Name:          admissionReviewReq.Request.Name,
			Namespace:     admissionReviewReq.Request.Namespace,
			Operation:     admissionregistrationv1.OperationType(admissionReviewReq.Request.Operation),
			UserInfo:      &admissionReviewReq.Request.UserInfo,
			Object:        admissionReviewReq.Request.Object.Raw,
			OldObject:     admissionReviewReq.Request.OldObject.Raw,
			DynamicClient: dc,
			APIClient:     apiClient,
		}

		// Generate admission response
		admissionResponse := webhookFunc(&admissionRequest)
		admissionReview.Response = responseV1ToV1beta1(admissionResponse)
		admissionReview.Response.UID = admissionReviewReq.Request.UID
		response = admissionReview
	default:
		log.Errorf("Group version kind %v is not supported", gvk)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	encoder := json.NewEncoder(w)
	err = encoder.Encode(&response)
	if err != nil {
		log.Warnf("Failed to encode the response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// responseV1ToV1beta1 converts a v1.AdmissionResponse into a v1beta1.AdmissionResponse.
func responseV1ToV1beta1(resp *admiv1.AdmissionResponse) *admiv1beta1.AdmissionResponse {
	var patchType *admiv1beta1.PatchType
	if resp.PatchType != nil {
		typ := admiv1beta1.PatchType(*resp.PatchType)
		patchType = &typ
	}

	return &admiv1beta1.AdmissionResponse{
		UID:              resp.UID,
		Allowed:          resp.Allowed,
		AuditAnnotations: resp.AuditAnnotations,
		Patch:            resp.Patch,
		PatchType:        patchType,
		Result:           resp.Result,
		Warnings:         resp.Warnings,
	}
}
