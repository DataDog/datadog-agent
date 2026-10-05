// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package azurefiles tests log collection from Azure Files across rotation,
// through a kernel CIFS mount.
package azurefiles

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	azureresources "github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	azurekubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/azure/kubernetes"
)

const (
	e2eNamespace          = "azure-files-e2e"
	agentNamespace        = "datadog"
	storageSecretPrefix   = "azure-files-storage"
	accountNameSecretKey  = "azurestorageaccountname"
	accountKeySecretKey   = "azurestorageaccountkey"
	writerContainerName   = "writer"
	ledgerContainerName   = "ledger"
	appenderContainerName = "appender"
	logMountPath          = "/mnt/azure-files"
	activeLogName         = "app.log"
	ledgerName            = "ledger.jsonl"
	writerTargetSequence  = "81792,82885,84060,81920,85000,82500"
	partOfLabel           = "app.kubernetes.io/part-of"
	partOfLabelValue      = "azure-files-e2e"
	runIDLabel            = "e2e.datadoghq.com/run-id"
	cellLabel             = "e2e.datadoghq.com/cell"
)

// Mount options are selected per cell. Every measurement so far used
// actimeo=1, while Microsoft recommends actimeo=30 to actimeo=60 for Azure
// Files on Linux and documents that lower values cost performance, so real
// deployments are far more likely to run 30 or more. Both are kept so their
// results stay comparable; only the attribute cache lifetime differs.
const (
	mountOptionsActimeo1  = "cache=strict,nosharesock,serverino,actimeo=1,closetimeo=0,persistenthandles"
	mountOptionsActimeo30 = "cache=strict,nosharesock,serverino,actimeo=30,closetimeo=0,persistenthandles"
)

// Agent settings this suite pins, and the drain windows they produce.
//
// Each reader keeps reading a rotated file for a bounded window after it sees
// the rotation, and drops whatever is appended once that window has closed:
//
//   - file source, default profile: the rotated tailer keeps reading next to
//     its replacement for logs_config.close_timeout, then closes its
//     descriptor;
//   - file source, unreliable-mount profile: the rotated tailer hands the path
//     over and stops once its file has gone fileHandoffQuietSeconds without new
//     reads (handoffQuietPeriod in pkg/logs/tailers/file/tailer.go), or at
//     logs_config.unreliable_mount.rotation_drain_timeout, 60s by default.
//
// close_timeout is lowered from its 60s default so that a single marker age
// can land past every window and still come before the next rotation.
const (
	fileScanPeriodSeconds   = 1
	closeTimeoutSeconds     = 5
	fileHandoffQuietSeconds = 30
	writerRotationSeconds   = 60
)

// Post-rename marker calibration.
//
// The appender sidecar writes one marker inside every drain window described
// above and one past all of them:
//
//   - the surviving marker must always be collected, because it lands while
//     every reader is still reading the rotated file;
//   - the lost marker is the calibration point. It is expected to disappear,
//     which is what proves this suite can observe the loss at all.
//
// The windows open when the Agent sees the rotation, not at the rename. A file
// source sees the rename after up to one file scan plus the mount's attribute
// cache lifetime, so with actimeo=30 the window of the file-line-actimeo30 cell
// can open late enough to cover the lost marker in the unreliable-mount
// profile. The lost marker cannot be moved later: the appender works inline
// and must finish before the writer rotates again.
//
// If the lost marker starts arriving, treat the run as suspicious rather than
// as a pass and investigate the harness before the product: it usually means
// the marker no longer lands past the drain window (the constants above
// drifted from the Agent, the rotation was detected late so both markers
// slipped, or the surviving marker kept the drain alive longer than expected).
const (
	postRotationMarkerSurvivingDelayMs = 1500
	postRotationMarkerLostDelayMs      = 45000
	postRotationMarkerSurvivingCount   = 1
	postRotationMarkerLostCount        = 0
	postRotationMarkerPollMs           = 200
	postRotationMarkerJournalName      = "markers.jsonl"
)

// readerKind is the Agent log source that collects a cell's share.
type readerKind string

const (
	// fileReader is a file log source over the Agent's own CIFS mount.
	fileReader readerKind = "file"
)

// fingerprintConfig is the per-source fingerprint_config of a file reader.
type fingerprintConfig struct {
	strategy string
	count    int
	maxBytes int
}

type cell struct {
	name        string
	reader      readerKind
	service     string
	fingerprint fingerprintConfig
	// mountOptions applies to the writer's mount, and to the Agent's mount
	// when the reader is a file source.
	mountOptions string
	shareName    string
	accountName  string
	volumeName   string
	writerName   string
}

// secretName is the Kubernetes Secret holding the cell's storage account
// credentials. The CSI driver reads it for the CIFS mounts.
func (c cell) secretName() string {
	return storageSecretPrefix + "-" + c.name
}

func (c cell) host() string {
	return c.accountName + ".file.core.windows.net"
}

// agentProfile is the node-wide Agent configuration of a run.
// logs_config.unreliable_mount applies to every file source on the Agent and
// the suite runs one Agent, so one run covers one profile and comparing the
// profiles takes one run each.
type agentProfile struct {
	name            string
	unreliableMount bool
}

var (
	defaultProfile         = agentProfile{name: "default"}
	unreliableMountProfile = agentProfile{name: "unreliable-mount", unreliableMount: true}
	knownProfiles          = []agentProfile{defaultProfile, unreliableMountProfile}
)

func parseProfile(name string) (agentProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultProfile, nil
	}
	names := make([]string, 0, len(knownProfiles))
	for _, profile := range knownProfiles {
		if profile.name == name {
			return profile, nil
		}
		names = append(names, profile.name)
	}
	return agentProfile{}, fmt.Errorf("unknown profile %q; known profiles are %s", name, strings.Join(names, ","))
}

type runSpec struct {
	runID       string
	stackName   string
	writerImage string
	profile     agentProfile
	cells       []cell
}

func newRunSpec(runID, writerImage, profileName, cellFilter string) (runSpec, error) {
	profile, err := parseProfile(profileName)
	if err != nil {
		return runSpec{}, err
	}

	sum := sha256.Sum256([]byte(runID))
	digest := hex.EncodeToString(sum[:])
	lineFingerprint := fingerprintConfig{strategy: "line_checksum", count: 1, maxBytes: 4096}
	spec := runSpec{
		runID:       runID,
		stackName:   "azure-files-" + digest[:8],
		writerImage: writerImage,
		profile:     profile,
		cells: []cell{
			{
				name:         "file-line",
				reader:       fileReader,
				fingerprint:  lineFingerprint,
				mountOptions: mountOptionsActimeo1,
				accountName:  "ddafline" + digest[:14],
				shareName:    "fline-" + digest[:12],
			},
			{
				name:         "file-byte",
				reader:       fileReader,
				fingerprint:  fingerprintConfig{strategy: "byte_checksum", count: 2048, maxBytes: 2048},
				mountOptions: mountOptionsActimeo1,
				accountName:  "ddafbyte" + digest[:14],
				shareName:    "fbyte-" + digest[:12],
			},
			{
				name:         "file-line-actimeo30",
				reader:       fileReader,
				fingerprint:  lineFingerprint,
				mountOptions: mountOptionsActimeo30,
				accountName:  "ddafln30" + digest[:14],
				shareName:    "fline30-" + digest[:12],
			},
		},
	}
	for i := range spec.cells {
		spec.cells[i].service = "azure-files-" + spec.cells[i].name
		spec.cells[i].volumeName = "azure-files-" + spec.cells[i].name
		spec.cells[i].writerName = "writer-" + spec.cells[i].name
	}

	selected, err := filterCells(spec.cells, cellFilter)
	if err != nil {
		return runSpec{}, err
	}
	spec.cells = selected
	return spec, nil
}

// filterCells keeps only the named cells, so a run can provision one cell
// instead of the whole matrix. An empty filter keeps every cell.
func filterCells(all []cell, cellFilter string) ([]cell, error) {
	wanted := make([]string, 0, len(all))
	for _, name := range strings.Split(cellFilter, ",") {
		if name = strings.TrimSpace(name); name != "" {
			wanted = append(wanted, name)
		}
	}
	if len(wanted) == 0 {
		return all, nil
	}

	selected := make([]cell, 0, len(wanted))
	for _, name := range wanted {
		found := false
		for _, c := range all {
			if c.name == name {
				selected = append(selected, c)
				found = true
				break
			}
		}
		if !found {
			names := make([]string, 0, len(all))
			for _, c := range all {
				names = append(names, c.name)
			}
			return nil, fmt.Errorf("unknown cell %q; known cells are %s", name, strings.Join(names, ","))
		}
	}
	return selected, nil
}

func postRotationMarkerDelaysMs() string {
	return fmt.Sprintf("%d,%d", postRotationMarkerSurvivingDelayMs, postRotationMarkerLostDelayMs)
}

// storageProvisioner creates AKS and the Azure Files resources without an
// Agent. The second UpdateEnv pass installs the Agent only after the CSI
// secrets and shares already exist on the stack.
func (spec runSpec) storageProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	return azurekubernetes.AKSProvisioner(
		azurekubernetes.WithName("azurefiles"),
		// Called without arguments, this sets the Agent options to nil, which
		// makes the provisioner skip the Agent. Its default, an empty non-nil
		// list, would install one.
		azurekubernetes.WithAgentOptions(),
		azurekubernetes.WithWorkloadApp(spec.storageWorkload),
	)
}

func (spec runSpec) agentProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	return azurekubernetes.AKSProvisioner(
		azurekubernetes.WithName("azurefiles"),
		azurekubernetes.WithAgentOptions(
			kubernetesagentparams.WithHelmValues(spec.agentHelmValues()),
			kubernetesagentparams.WithoutLogsContainerCollectAll(),
		),
		azurekubernetes.WithAgentDependentWorkloadApp(spec.writerWorkload),
		azurekubernetes.WithWorkloadApp(spec.storageWorkload),
	)
}

type azureStorageAccount struct {
	pulumi.CustomResourceState
	Name pulumi.StringOutput `pulumi:"name"`
}

type azureFileShare struct {
	pulumi.CustomResourceState
}

// storageWorkload creates one storage account and file share per cell through
// the generic azure-native resource tokens, so the suite needs no storage SDK
// module.
func (spec runSpec) storageWorkload(env config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
	azureEnv, ok := env.(*azureresources.Environment)
	if !ok {
		return nil, fmt.Errorf("Azure Files workload requires *azure.Environment, got %T", env)
	}

	ctx := env.Ctx()
	workload := &kubecomp.Workload{}
	if err := ctx.RegisterComponentResource("dd:e2e:AzureFilesStorage", spec.runID, workload); err != nil {
		return nil, err
	}

	kubeOpts := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
		pulumi.Parent(workload),
		pulumi.DeletedWith(kubeProvider),
	}
	namespace, err := corev1.NewNamespace(ctx, e2eNamespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(e2eNamespace),
			Labels: pulumi.StringMap{
				partOfLabel: pulumi.String(partOfLabelValue),
				runIDLabel:  pulumi.String(spec.runID),
			},
		},
	}, kubeOpts...)
	if err != nil {
		return nil, err
	}
	kubeOpts = append(kubeOpts, utils.PulumiDependsOn(namespace))

	if env.ImagePullRegistry() != "" {
		if _, err := utils.NewImagePullSecret(env, e2eNamespace, kubeOpts...); err != nil {
			return nil, err
		}
	}

	for _, c := range spec.cells {
		account := &azureStorageAccount{}
		err := ctx.RegisterResource("azure-native:storage:StorageAccount", c.accountName, pulumi.Map{
			"accountName":       pulumi.String(c.accountName),
			"kind":              pulumi.String("StorageV2"),
			"resourceGroupName": pulumi.String(azureEnv.DefaultResourceGroup()),
			"sku":               pulumi.Map{"name": pulumi.String("Standard_LRS")},
			"tags":              env.ResourcesTags(),
		}, account, pulumi.Parent(workload), azureEnv.WithProviders(config.ProviderAzure))
		if err != nil {
			return nil, err
		}

		share := &azureFileShare{}
		err = ctx.RegisterResource("azure-native:storage:FileShare", c.shareName, pulumi.Map{
			"accountName":       account.Name,
			"resourceGroupName": pulumi.String(azureEnv.DefaultResourceGroup()),
			"shareName":         pulumi.String(c.shareName),
			"shareQuota":        pulumi.Int(5),
		}, share, pulumi.Parent(workload), azureEnv.WithProviders(config.ProviderAzure), pulumi.DependsOn([]pulumi.Resource{account}))
		if err != nil {
			return nil, err
		}

		keys := ctx.InvokeOutput(
			"azure-native:storage:listStorageAccountKeys",
			pulumi.Map{
				"accountName":       account.Name,
				"resourceGroupName": pulumi.String(azureEnv.DefaultResourceGroup()),
			},
			pulumi.MapOutput{},
			pulumi.InvokeOutputOptions{InvokeOptions: []pulumi.InvokeOption{
				azureEnv.WithProvider(config.ProviderAzure),
				pulumi.DependsOn([]pulumi.Resource{account}),
			}},
		).(pulumi.MapOutput)
		accountKey := keys.MapIndex(pulumi.String("keys")).ApplyT(firstStorageAccountKey).(pulumi.StringOutput)
		storageSecretOpts := append([]pulumi.ResourceOption{}, kubeOpts...)
		storageSecretOpts = append(storageSecretOpts, utils.PulumiDependsOn(share))
		_, err = corev1.NewSecret(ctx, c.secretName(), &corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(c.secretName()),
				Namespace: pulumi.String(e2eNamespace),
			},
			StringData: pulumi.StringMap{
				accountNameSecretKey: account.Name,
				accountKeySecretKey:  pulumi.ToSecret(accountKey).(pulumi.StringOutput),
			},
		}, storageSecretOpts...)
		if err != nil {
			return nil, err
		}
	}

	if err := ctx.RegisterResourceOutputs(workload, pulumi.Map{
		"namespace": pulumi.String(e2eNamespace),
		"runID":     pulumi.String(spec.runID),
	}); err != nil {
		return nil, err
	}
	return workload, nil
}

func firstStorageAccountKey(value any) (string, error) {
	keys, ok := value.([]any)
	if !ok || len(keys) == 0 {
		return "", errors.New("Azure returned no storage account keys")
	}
	key, ok := keys[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("Azure returned an unexpected storage account key type %T", keys[0])
	}
	keyValue, ok := key["value"].(string)
	if !ok || keyValue == "" {
		return "", errors.New("Azure returned an empty storage account key")
	}
	return keyValue, nil
}

func (spec runSpec) writerWorkload(
	env config.Env,
	kubeProvider *kubernetes.Provider,
	dependsOnAgent pulumi.ResourceOption,
) (*kubecomp.Workload, error) {
	ctx := env.Ctx()
	workload := &kubecomp.Workload{}
	if err := ctx.RegisterComponentResource("dd:e2e:AzureFilesWriters", spec.runID, workload); err != nil {
		return nil, err
	}

	kubeOpts := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
		pulumi.Parent(workload),
		pulumi.DeletedWith(kubeProvider),
		dependsOnAgent,
	}
	for _, c := range spec.cells {
		if _, err := spec.newWriterDeployment(env, c, kubeOpts); err != nil {
			return nil, err
		}
	}

	if err := ctx.RegisterResourceOutputs(workload, pulumi.Map{
		"namespace": pulumi.String(e2eNamespace),
		"runID":     pulumi.String(spec.runID),
	}); err != nil {
		return nil, err
	}
	return workload, nil
}

// writerRunID identifies one cell of one run in the writer's records, its
// ledger, and its markers.
func (spec runSpec) writerRunID(c cell) string {
	return spec.runID + "-" + c.name
}

func (spec runSpec) newWriterDeployment(
	env config.Env,
	c cell,
	baseOpts []pulumi.ResourceOption,
) (*appsv1.Deployment, error) {
	labels := pulumi.StringMap{
		"app.kubernetes.io/name": pulumi.String(c.writerName),
		partOfLabel:              pulumi.String(partOfLabelValue),
		runIDLabel:               pulumi.String(spec.runID),
		cellLabel:                pulumi.String(c.name),
	}
	volume := corev1.VolumeArgs{
		Name: pulumi.String(c.volumeName),
		Csi: corev1.CSIVolumeSourceArgs{
			Driver:   pulumi.String("file.csi.azure.com"),
			ReadOnly: pulumi.Bool(false),
			VolumeAttributes: pulumi.StringMap{
				"storageAccount":  pulumi.String(c.accountName),
				"server":          pulumi.String(c.host()),
				"shareName":       pulumi.String(c.shareName),
				"secretName":      pulumi.String(c.secretName()),
				"secretNamespace": pulumi.String(e2eNamespace),
				"mountOptions":    pulumi.String(c.mountOptions),
			},
		},
	}
	volumeMount := corev1.VolumeMountArgs{
		Name:      pulumi.String(c.volumeName),
		MountPath: pulumi.String(logMountPath),
	}
	envVars := corev1.EnvVarArray{
		corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_LOG_DIR"), Value: pulumi.String(logMountPath)},
		corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_RUN_ID"), Value: pulumi.String(spec.writerRunID(c))},
		corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_TARGET_BYTES_SEQUENCE"), Value: pulumi.String(writerTargetSequence)},
		corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_HEAD_PAUSE_MS"), Value: pulumi.String("5000")},
		corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_MAX_RECORDS_PER_PERIOD"), Value: pulumi.String("5000")},
		corev1.EnvVarArgs{Name: pulumi.String("TZ"), Value: pulumi.String("UTC")},
		corev1.EnvVarArgs{Name: pulumi.String("JAVA_TOOL_OPTIONS"), Value: pulumi.String("-Duser.timezone=UTC")},
	}
	podSpec := &corev1.PodSpecArgs{
		NodeSelector: pulumi.StringMap{
			"kubernetes.io/arch": pulumi.String("amd64"),
			"kubernetes.io/os":   pulumi.String("linux"),
		},
		Containers: corev1.ContainerArray{
			corev1.ContainerArgs{
				Name:            pulumi.String(writerContainerName),
				Image:           pulumi.String(spec.writerImage),
				ImagePullPolicy: pulumi.String("IfNotPresent"),
				Env:             envVars,
				VolumeMounts:    corev1.VolumeMountArray{volumeMount},
				ReadinessProbe: corev1.ProbeArgs{
					Exec: corev1.ExecActionArgs{Command: pulumi.ToStringArray([]string{
						"/bin/sh", "-c", fmt.Sprintf("test $(wc -c < %s/%s) -ge 4096", logMountPath, activeLogName),
					})},
					PeriodSeconds:    pulumi.Int(2),
					FailureThreshold: pulumi.Int(45),
				},
			},
			corev1.ContainerArgs{
				Name:            pulumi.String(ledgerContainerName),
				Image:           pulumi.String(spec.writerImage),
				ImagePullPolicy: pulumi.String("IfNotPresent"),
				Command:         pulumi.ToStringArray([]string{"/bin/sh", "/app/ledger.sh"}),
				Env: corev1.EnvVarArray{
					corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_LOG_DIR"), Value: pulumi.String(logMountPath)},
				},
				VolumeMounts: corev1.VolumeMountArray{volumeMount},
			},
			// The appender shares the writer pod, so it appends through the
			// same CIFS mount that performed the rename. The rotation is
			// therefore visible to it immediately, whatever actimeo the cell
			// uses for revalidating remotely changed metadata.
			corev1.ContainerArgs{
				Name:            pulumi.String(appenderContainerName),
				Image:           pulumi.String(spec.writerImage),
				ImagePullPolicy: pulumi.String("IfNotPresent"),
				Command:         pulumi.ToStringArray([]string{"/bin/sh", "/app/appender.sh"}),
				Env: corev1.EnvVarArray{
					corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_LOG_DIR"), Value: pulumi.String(logMountPath)},
					corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_RUN_ID"), Value: pulumi.String(spec.writerRunID(c))},
					corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_MARKER_JOURNAL_PATH"), Value: pulumi.String(logMountPath + "/" + postRotationMarkerJournalName)},
					corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_APPEND_DELAYS_MS"), Value: pulumi.String(postRotationMarkerDelaysMs())},
					corev1.EnvVarArgs{Name: pulumi.String("LOGWRITER_APPEND_POLL_MS"), Value: pulumi.String(strconv.Itoa(postRotationMarkerPollMs))},
					corev1.EnvVarArgs{Name: pulumi.String("TZ"), Value: pulumi.String("UTC")},
				},
				VolumeMounts: corev1.VolumeMountArray{volumeMount},
			},
		},
		Volumes: corev1.VolumeArray{volume},
	}
	if env.ImagePullRegistry() != "" {
		podSpec.ImagePullSecrets = corev1.LocalObjectReferenceArray{
			corev1.LocalObjectReferenceArgs{Name: pulumi.String(utils.DefaultImagePullSecretName)},
		}
	}

	opts := append([]pulumi.ResourceOption{}, baseOpts...)
	return appsv1.NewDeployment(env.Ctx(), c.writerName, &appsv1.DeploymentArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String(c.writerName),
			Namespace: pulumi.String(e2eNamespace),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: metav1.LabelSelectorArgs{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategyArgs{Type: pulumi.String("Recreate")},
			Template: corev1.PodTemplateSpecArgs{
				Metadata: metav1.ObjectMetaArgs{Labels: labels},
				Spec:     podSpec,
			},
		},
	}, opts...)
}

// agentMountPath is where the Agent mounts the share of a file reader cell.
func agentMountPath(c cell) string {
	return logMountPath + "/" + c.name
}

func (spec runSpec) agentHelmValues() string {
	var sources strings.Builder
	var volumes strings.Builder
	var mounts strings.Builder
	for _, c := range spec.cells {
		switch c.reader {
		case fileReader:
			fmt.Fprintf(&sources, `      - type: file
        path: %[1]s/%[2]s
        exclude_paths:
          - %[1]s/%[2]s.*
        service: %[3]s
        source: java
        start_position: beginning
        fingerprint_config:
          fingerprint_strategy: %[4]s
          count: %[5]d
          count_to_skip: 0
          max_bytes: %[6]d
`, agentMountPath(c), activeLogName, c.service, c.fingerprint.strategy, c.fingerprint.count, c.fingerprint.maxBytes)
			fmt.Fprintf(&volumes, `    - name: %s
      csi:
        driver: file.csi.azure.com
        readOnly: true
        volumeAttributes:
          storageAccount: %s
          server: %s
          shareName: %s
          secretName: %s
          secretNamespace: %s
          mountOptions: %q
`, c.volumeName, c.accountName, c.host(), c.shareName, c.secretName(), e2eNamespace, c.mountOptions)
			fmt.Fprintf(&mounts, `    - name: %s
      mountPath: %s
      readOnly: true
`, c.volumeName, agentMountPath(c))
		}
		fmt.Fprintf(&sources, `        tags:
          - e2e_run_id:%s
          - e2e_cell:%s
          - e2e_profile:%s
`, spec.runID, c.name, spec.profile.name)
	}

	var values strings.Builder
	fmt.Fprintf(&values, `datadog:
  logs:
    enabled: true
    containerCollectAll: false
  logLevel: DEBUG
  env:
    - name: DD_LOGS_CONFIG_FILE_SCAN_PERIOD
      value: "%d"
    - name: DD_LOGS_CONFIG_CLOSE_TIMEOUT
      value: "%d"
    - name: DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED
      value: "%t"
`, fileScanPeriodSeconds, closeTimeoutSeconds, spec.profile.unreliableMount)
	fmt.Fprintf(&values, `  confd:
    azure_files.yaml: |-
      logs:
%s`, sources.String())
	if volumes.Len() > 0 {
		fmt.Fprintf(&values, `agents:
  volumes:
%s  volumeMounts:
%s`, volumes.String(), mounts.String())
	}
	return values.String()
}
