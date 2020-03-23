package admissioncontroller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/kubernetes/pkg/apis/core/v1"

	"github.com/DataDog/datadog-agent/cmd/agent/common/jsonpatch"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
)

func init() {
	corev1.AddToScheme(runtimeScheme)
	admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	// defaulting with webhooks:
	// https://github.com/kubernetes/kubernetes/issues/57982
	v1.AddToScheme(runtimeScheme)
}

func (whsvr *WebhookServer) status(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("status OK"))
}

func (whsvr *WebhookServer) serve(w http.ResponseWriter, r *http.Request) {
	log.Error("test version 1") // TODO remove debug statement
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	log.Debug("admission controller request body: %v", body)

	requestedAdmissionReview := v1beta1.AdmissionReview{}
	responseAdmissionReview := v1beta1.AdmissionReview{}

	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		responseAdmissionReview.Response = newAdmissionResponseWithError(err)
	} else {
		if requestedAdmissionReview.Request == nil {
			w.WriteHeader(http.StatusBadRequest)
			log.Info("received empty request")
			return
		}

		responseAdmissionReview.Response = handleAdmissionReview(requestedAdmissionReview)
	}

	// Return the same UID
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	log.Debug("admission controller response: %v", responseAdmissionReview.Response)

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Error(err)
		return
	}
	if _, err := w.Write(respBytes); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Error(err)
		return
	}
}

// Returns a simple response with the provided error.
// The webhook is still considered as allowed, in order
// to not interfere with the user's deployment.
func newAdmissionResponseWithMessage(message string, params ...interface{}) *v1beta1.AdmissionResponse {
	msg := fmt.Sprintf(message, params...)
	log.Error(msg)
	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: msg,
		},
	}
}

func newAdmissionResponseWithError(err error) *v1beta1.AdmissionResponse {
	return newAdmissionResponseWithMessage(err.Error())
}

var (
	podResource = metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
)

func handleAdmissionReview(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	if ar.Request == nil {
		return newAdmissionResponseWithMessage("empty resource %v not supported", ar)
	}

	switch {
	case ar.Request.Resource == podResource:
		return handlePodRequest(ar.Request.Object.Raw)
	default:
		return newAdmissionResponseWithMessage("resource %v not supported", ar.Request.Resource)
	}
}

func handlePodRequest(raw []byte) *v1beta1.AdmissionResponse {
	pod := corev1.Pod{}
	if err := json.Unmarshal(raw, &pod); err != nil {
		return newAdmissionResponseWithError(err)
	}

	patch := mutatePod(pod, getEnvMutator())
	return newJSONPatchResponse(patch)
}

var (
	patchTypeJSONPatch = v1beta1.PatchTypeJSONPatch
)

func newJSONPatchResponse(patch jsonpatch.Patch) *v1beta1.AdmissionResponse {
	bytes, err := json.Marshal(patch)
	if err != nil {
		return newAdmissionResponseWithError(err)
	}

	return &v1beta1.AdmissionResponse{
		Allowed:   true,
		Patch:     bytes,
		PatchType: &patchTypeJSONPatch,
	}
}

// NewEnvMutator creates a new mutator which adds environment
// variables to pods
func getEnvMutator() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "DD_AGENT_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.hostIP",
				},
			},
		},
	}
}

func mutatePod(pod corev1.Pod, envMutator []corev1.EnvVar) jsonpatch.Patch {
	containerLists := []struct {
		containerType string
		containers    []corev1.Container
	}{
		{"initContainers", pod.Spec.InitContainers},
		{"containers", pod.Spec.Containers},
	}

	patch := jsonpatch.Patch{}

	for _, s := range containerLists {
		containerType, containers := s.containerType, s.containers
		for i, container := range containers {
			// If Container does not have any environment variables,
			// create the base path first with an empty array.
			if len(container.Env) == 0 {
				patch = append(patch, jsonpatch.Add(
					fmt.Sprint("/spec/", containerType, "/", i, "/env"),
					[]interface{}{},
				))
			}

		mutateEnv:
			for envPos, def := range envMutator {
				// Skip current mutation if the variable already exists.
				// We do not want to override provided by the user.
				for _, v := range container.Env {
					if v.Name == def.Name {
						continue mutateEnv
					}
				}

				patch = append(patch, jsonpatch.Add(
					fmt.Sprint("/spec/", containerType, "/", i, "/env/", envPos),
					def,
				))
			}
		}
	}
	return patch
}
