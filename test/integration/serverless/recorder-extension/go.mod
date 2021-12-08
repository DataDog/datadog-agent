module datadog-lambda-extension/recorder-extension

go 1.16

replace (
	github.com/iovisor/gobpf => github.com/DataDog/gobpf v0.0.0-20210322155958-9866ef4cd22c
	k8s.io/api => k8s.io/api v0.20.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.5
	k8s.io/apiserver => k8s.io/apiserver v0.20.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.5
	k8s.io/client-go => k8s.io/client-go v0.20.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.5
	k8s.io/code-generator => k8s.io/code-generator v0.20.5
	k8s.io/component-base => k8s.io/component-base v0.20.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.5
	k8s.io/cri-api => k8s.io/cri-api v0.20.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.5
	k8s.io/kubectl => k8s.io/kubectl v0.20.5
	k8s.io/kubelet => k8s.io/kubelet v0.20.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.5
	k8s.io/metrics => k8s.io/metrics v0.20.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.3-rc.0
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.5
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.20.5
	k8s.io/sample-controller => k8s.io/sample-controller v0.20.5
)

require (
	github.com/Azure/go-autorest/autorest v0.11.18 // indirect
	github.com/DataDog/agent-payload v4.89.0+incompatible
	github.com/DataDog/datadog-agent v0.0.0-20211208145626-871818bed028
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.0.0-20211201172000-1fd9a353e8e4 // indirect
	github.com/alecthomas/units v0.0.0-20210208195552-ff826a37aa15 // indirect
	github.com/cobaugh/osrelease v0.0.0-20181218015638-a93a0a55a249 // indirect
	github.com/go-kit/kit v0.10.0 // indirect
	github.com/ttacon/chalk v0.0.0-20160626202418-22c06c80ed31 // indirect
	honnef.co/go/tools v0.1.1 // indirect
	k8s.io/client-go v12.0.0+incompatible // indirect
	k8s.io/kubernetes v1.20.5 // indirect
)
