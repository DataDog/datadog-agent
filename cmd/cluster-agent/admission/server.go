// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	admiv1 "k8s.io/api/admission/v1"
	admiv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const jsonContentType = "application/json"

type admissionFunc func([]byte, string, dynamic.Interface) ([]byte, error)

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
func (s *Server) Register(uri string, f admissionFunc, dc dynamic.Interface) {
	s.mux.HandleFunc(uri, func(w http.ResponseWriter, r *http.Request) {
		s.mutateHandler(w, r, f, dc)
	})
}

// Run starts the kubernetes admission webhook server.
func (s *Server) Run(mainCtx context.Context, client kubernetes.Interface) error {
	var tlsMinVersion uint16 = tls.VersionTLS13
	if config.Datadog.GetBool("cluster_agent.allow_legacy_tls") {
		tlsMinVersion = tls.VersionTLS10
	}

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Datadog.GetInt("admission_controller.port")),
		Handler: s.mux,
		TLSConfig: &tls.Config{
			GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
				secretNs := common.GetResourcesNamespace()
				secretName := config.Datadog.GetString("admission_controller.certificate.secret_name")
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

// mutateHandler contains the main logic responsible for handling mutation requests.
// It supports both v1 and v1beta1 requests.
func (s *Server) mutateHandler(w http.ResponseWriter, r *http.Request, mutateFunc admissionFunc, dc dynamic.Interface) {
	metrics.WebhooksReceived.Inc()

	start := time.Now()
	defer func() {
		metrics.WebhooksResponseDuration.Observe(time.Since(start).Seconds())
	}()

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

	obj, gvk, err := s.decoder.Decode(body, nil, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Warnf("Could not deserialize request: %v", err)
		return
	}

	var response runtime.Object
	switch *gvk {
	case admiv1.SchemeGroupVersion.WithKind("AdmissionReview"):
		admissionReviewReq, ok := obj.(*admiv1.AdmissionReview)
		if !ok {
			log.Errorf("Expected v1.AdmissionReview, got type %T", obj)
		}
		admissionReviewResp := &admiv1.AdmissionReview{}
		admissionReviewResp.SetGroupVersionKind(*gvk)
		jsonPatch, err := mutateFunc(admissionReviewReq.Request.Object.Raw, admissionReviewReq.Request.Namespace, dc)
		admissionReviewResp.Response = mutationResponse(jsonPatch, err)
		admissionReviewResp.Response.UID = admissionReviewReq.Request.UID
		response = admissionReviewResp
	case admiv1beta1.SchemeGroupVersion.WithKind("AdmissionReview"):
		admissionReviewReq, ok := obj.(*admiv1beta1.AdmissionReview)
		if !ok {
			log.Errorf("Expected v1beta1.AdmissionReview, got type %T", obj)
		}
		admissionReviewResp := &admiv1beta1.AdmissionReview{}
		admissionReviewResp.SetGroupVersionKind(*gvk)
		jsonPatch, err := mutateFunc(admissionReviewReq.Request.Object.Raw, admissionReviewReq.Request.Namespace, dc)
		admissionReviewResp.Response = responseV1ToV1beta1(mutationResponse(jsonPatch, err))
		admissionReviewResp.Response.UID = admissionReviewReq.Request.UID
		response = admissionReviewResp
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

// mutationResponse returns the adequate v1.AdmissionResponse based on the mutation result.
func mutationResponse(jsonPatch []byte, err error) *admiv1.AdmissionResponse {
	if err != nil {
		log.Warnf("Failed to mutate: %v", err)

		return &admiv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
			Allowed: true,
		}

	}

	patchType := admiv1.PatchTypeJSONPatch

	return &admiv1.AdmissionResponse{
		Patch:     jsonPatch,
		PatchType: &patchType,
		Allowed:   true,
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
