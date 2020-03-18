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
	deserializer  = codecs.UniversalDeserializer()

	// (https://github.com/kubernetes/kubernetes/issues/57982)
	defaulter = runtime.ObjectDefaulter(runtimeScheme)
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
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Errorf("contentType=%v, expect application/json", contentType)
		return
	}

	log.Debug("Admission controller request body: %v", body)

	// The AdmissionReview that was sent to the admissioncontroller
	requestedAdmissionReview := v1beta1.AdmissionReview{}

	// The AdmissionReview that will be returned
	responseAdmissionReview := v1beta1.AdmissionReview{}

	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		log.Error(err)
		responseAdmissionReview.Response = toAdmissionResponse(err)
	} else {
		// pass to admitFunc
		responseAdmissionReview.Response = mutatePods(requestedAdmissionReview)
	}

	// Return the same UID
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	log.Debug("Admission controller response: %v", responseAdmissionReview.Response)

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		log.Error(err)
	}
	if _, err := w.Write(respBytes); err != nil {
		log.Error(err)
	}
}

// toAdmissionResponse is a helper function to create an AdmissionResponse
// with an embedded error
func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func mutatePods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if ar.Request.Resource != podResource {
		log.Errorf("expect resource to be %s, got %v", podResource, ar.Request.Resource)

		return &v1beta1.AdmissionResponse{Allowed: true}
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		log.Error(err)
		return toAdmissionResponse(err)
	}

	patch, _ := mutatePod(pod)
	return mutateResponse(patch)
}

func mutateResponse(patch jsonpatch.Patch) *v1beta1.AdmissionResponse {
	bs, _ := json.Marshal(patch)
	patchType := v1beta1.PatchTypeJSONPatch
	return &v1beta1.AdmissionResponse{
		Allowed:   true,
		Patch:     bs,
		PatchType: &patchType,
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

func mutatePod(pod corev1.Pod) (jsonpatch.Patch, error) {
	var envVariables = getEnvMutator()

	containerLists := []struct {
		field      string
		containers []corev1.Container
	}{
		{"initContainers", pod.Spec.InitContainers},
		{"containers", pod.Spec.Containers},
	}

	var patch jsonpatch.Patch

	for _, s := range containerLists {
		field, containers := s.field, s.containers
		for i, container := range containers {
			if len(container.Env) == 0 {
				patch = append(patch, jsonpatch.Add(
					fmt.Sprint("/spec/", field, "/", i, "/env"),
					[]interface{}{},
				))
			}

			remainingEnv := make([]corev1.EnvVar, len(container.Env))
			copy(remainingEnv, container.Env)

		injectedEnvLoop:
			for envPos, def := range envVariables {
				for pos, v := range remainingEnv {
					if v.Name == def.Name {
						if currPos, destPos := envPos+pos, envPos; currPos != destPos {
							// This should ideally be a `move` operation but due to a bug in the json-patch's
							// implementation of `move` operation, we explicitly use `remove` followed by `add`.
							// see, https://github.com/evanphx/json-patch/pull/73
							// This is resolved in json-patch `v4.2.0`, which is pulled by Kubernetes `1.14.3` clusters.
							// https://github.com/kubernetes/kubernetes/blob/v1.14.3/Godeps/Godeps.json#L1707-L1709
							// TODO: Use a `move` operation, once all clusters are on `1.14.3+`
							patch = append(patch,
								jsonpatch.Remove(
									fmt.Sprint("/spec/", field, "/", i, "/env/", currPos),
								),
								jsonpatch.Add(
									fmt.Sprint("/spec/", field, "/", i, "/env/", destPos),
									v,
								))
						}
						remainingEnv = append(remainingEnv[:pos], remainingEnv[pos+1:]...)
						continue injectedEnvLoop
					}
				}

				patch = append(patch, jsonpatch.Add(
					fmt.Sprint("/spec/", field, "/", i, "/env/", envPos),
					def,
				))
			}
		}
	}
	return patch, nil
}
