// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package azurefiles tests log collection from Azure Files across rotation,
// through a kernel CIFS mount and through the native SMB log source.
package azurefiles

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
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
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/fakeintake"
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
	agentSecretsMountRoot = "/etc/azure-files-secrets"
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

// The writer pods run the stock image below unless a run names its own image.
// The stock image has Python and a POSIX shell but no Java, so the writer is
// workload/logwriter.py, a port of the Java writer that keeps its record,
// rotation and checksum formats, and the writer, the ledger and the appender
// come from a ConfigMap mounted where the Java image keeps its own copies.
const (
	// stockWorkloadImage is a Docker Hub image, pulled through the
	// environment's Docker Hub mirror; on Azure that is Docker Hub itself. The
	// digest pins the multi-arch index of the tag.
	stockWorkloadImage    = "library/python:3.12.15-slim-bookworm@sha256:54c85f3c47607a77f32adec749d3c81d1348bf25833671f512b26a9b6d778cb3"
	workloadConfigMapName = "azure-files-workload"
	workloadVolumeName    = "workload"
	workloadDir           = "/app"
	pythonWriterScript    = "logwriter.py"
	ledgerScript          = "ledger.sh"
	appenderScript        = "appender.sh"
	// stockWorkloadUser runs the stock containers. The Java image runs as a
	// user of its own; neither needs root on the CIFS mount.
	stockWorkloadUser = 1000
	// workloadChecksumAnnotation changes the writer pod template whenever the
	// ConfigMap scripts change, so a reused stack restarts the writers.
	workloadChecksumAnnotation = "e2e.datadoghq.com/workload-sha256"
)

var (
	//go:embed workload/logwriter.py
	pythonWriterSource string
	//go:embed workload/ledger.sh
	ledgerSource string
	//go:embed workload/appender.sh
	appenderSource string
)

// workloadKind names where the writer, ledger and appender come from.
type workloadKind string

const (
	// stockWorkload runs the ConfigMap scripts on stockWorkloadImage.
	stockWorkload workloadKind = "python"
	// customWorkload runs the Java writer image built from workload/, which
	// carries its own scripts.
	customWorkload workloadKind = "java"
)

type envVar struct {
	name  string
	value string
}

// workloadRuntime is how the three containers of a writer pod run.
type workloadRuntime struct {
	kind  workloadKind
	image string
	// writerCommand is nil when the image entrypoint runs the writer.
	writerCommand   []string
	ledgerCommand   []string
	appenderCommand []string
	// writerEnv and ledgerEnv are appended to those containers' environment.
	writerEnv []envVar
	ledgerEnv []envVar
	// scripts are the files of the workload ConfigMap, by file name, mounted
	// at workloadDir. They are empty when the image carries the scripts.
	scripts map[string]string
}

// workloadKind is the Java writer image when the run names one, and the stock
// image otherwise.
func (spec runSpec) workloadKind() workloadKind {
	if spec.writerImage != "" {
		return customWorkload
	}
	return stockWorkload
}

func (spec runSpec) workloadRuntime(dockerhubMirror string) workloadRuntime {
	ledgerCommand := []string{"/bin/sh", workloadDir + "/" + ledgerScript}
	appenderCommand := []string{"/bin/sh", workloadDir + "/" + appenderScript}
	if spec.workloadKind() == customWorkload {
		return workloadRuntime{
			kind:            customWorkload,
			image:           spec.writerImage,
			ledgerCommand:   ledgerCommand,
			appenderCommand: appenderCommand,
			writerEnv:       []envVar{{"JAVA_TOOL_OPTIONS", "-Duser.timezone=UTC"}},
		}
	}
	// The writer is the container's command, so it is PID 1 like the JVM of
	// the Java image, and its records carry the same %pid.
	writer := workloadDir + "/" + pythonWriterScript
	return workloadRuntime{
		kind:            stockWorkload,
		image:           dockerhubMirror + "/" + stockWorkloadImage,
		writerCommand:   []string{"python3", writer},
		ledgerCommand:   ledgerCommand,
		appenderCommand: appenderCommand,
		ledgerEnv:       []envVar{{"LOGWRITER_CRC64_COMMAND", "python3 " + writer + " crc64"}},
		scripts: map[string]string{
			pythonWriterScript: pythonWriterSource,
			ledgerScript:       ledgerSource,
			appenderScript:     appenderSource,
		},
	}
}

// scriptsChecksum digests the ConfigMap scripts.
func (r workloadRuntime) scriptsChecksum() string {
	names := make([]string, 0, len(r.scripts))
	for name := range r.scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		fmt.Fprintf(digest, "%s\x00%d\x00%s", name, len(r.scripts[name]), r.scripts[name])
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// Fakeintake keeps 15 minutes of payloads by default, which is shorter than a
// full matrix: the last cell checks logs written more than 15 minutes earlier.
// It also forwards everything to a Datadog org by default; a run that leaks
// the SMB account key into a log must not copy it anywhere else.
const fakeintakeRetention = "2h"

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
//     logs_config.unreliable_mount.rotation_drain_timeout, 60s by default;
//   - SMB source: the draining reader stops once smbDrainIdlePolls polls in a
//     row find no new data, or at logs_config.close_timeout. The scan that
//     detects the rotation already polls the drain, and that poll counts as
//     the first idle one (drainCaughtUpPolls in pkg/logs/launchers/smb). A
//     rotated file that stopped growing is therefore dropped one poll interval
//     after the Agent sees the rotation, which is the shortest window here.
//
// close_timeout is lowered from its 60s default so that a single marker age
// can land past every window and still come before the next rotation.
const (
	fileScanPeriodSeconds   = 1
	closeTimeoutSeconds     = 5
	fileHandoffQuietSeconds = 30
	smbPollIntervalSeconds  = 1
	smbDrainIdlePolls       = 2
	writerRotationSeconds   = 60
)

// Post-rename marker calibration.
//
// The appender sidecar of each cell writes one marker inside its reader's drain
// window and one past every window described above:
//
//   - the surviving marker must always be collected, because it lands while
//     the reader is still reading the rotated file. A file source reads the
//     rotated file for at least close_timeout, so its marker comes at 1.5s.
//     The SMB source can stop one poll interval after it sees the rotation,
//     and it sees the rotation at the earliest right after the rename, so its
//     marker comes within half a poll interval of the rename. At 1.5s it would
//     be lost on about half of the rotations, depending on where the rename
//     falls between two polls;
//   - the lost marker is the calibration point. It is expected to disappear,
//     which is what proves this suite can observe the loss at all.
//
// Every read of new data restarts the SMB source's idle count, so a cell's
// surviving marker also keeps its drain open: one cell cannot probe a later
// age with a third marker.
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
	fileSurvivingMarkerDelayMs       = 1500
	smbSurvivingMarkerDelayMs        = smbPollIntervalSeconds * 1000 / 2
	postRotationMarkerLostDelayMs    = 45000
	postRotationMarkerSurvivingCount = 1
	postRotationMarkerLostCount      = 0
	postRotationMarkerPollMs         = 200
	postRotationMarkerJournalName    = "markers.jsonl"
)

// markerDelays are the ages, after the rename, at which a cell's appender
// appends its two markers to the rotated file.
type markerDelays struct {
	survivingMs int
	lostMs      int
}

// markerDelaysFor returns the marker ages calibrated for a reader's drain
// window.
func markerDelaysFor(reader readerKind) markerDelays {
	if reader == smbReader {
		return markerDelays{survivingMs: smbSurvivingMarkerDelayMs, lostMs: postRotationMarkerLostDelayMs}
	}
	return markerDelays{survivingMs: fileSurvivingMarkerDelayMs, lostMs: postRotationMarkerLostDelayMs}
}

// appenderValue is the LOGWRITER_APPEND_DELAYS_MS value of the appender.
func (m markerDelays) appenderValue() string {
	return fmt.Sprintf("%d,%d", m.survivingMs, m.lostMs)
}

// readerKind is the Agent log source that collects a cell's share.
type readerKind string

const (
	// fileReader is a file log source over the Agent's own CIFS mount.
	fileReader readerKind = "file"
	// smbReader is the native SMB log source, which reads the share directly.
	smbReader readerKind = "smb"
)

// fingerprintConfig is the per-source fingerprint_config of a file reader.
type fingerprintConfig struct {
	strategy string
	count    int
	maxBytes int
}

// cell is one independent measurement. Each cell has its own storage account,
// share and writer, so no two readers ever see the same files.
type cell struct {
	name        string
	reader      readerKind
	service     string
	fingerprint fingerprintConfig
	// mountOptions applies to the writer's mount, and to the Agent's mount
	// when the reader is a file source.
	mountOptions string
	markers      markerDelays
	shareName    string
	accountName  string
	volumeName   string
	writerName   string
}

// secretName is the Kubernetes Secret holding the cell's storage account
// credentials. The CSI driver reads it in the e2e namespace for the CIFS
// mounts. For an SMB cell, a copy of the key under the same name in the Agent
// namespace is mounted into the Agent pod.
func (c cell) secretName() string {
	return storageSecretPrefix + "-" + c.name
}

func (c cell) host() string {
	return c.accountName + ".file.core.windows.net"
}

// agentKeyVolumeName is the Agent pod volume that holds an SMB cell's key.
func (c cell) agentKeyVolumeName() string {
	return c.volumeName + "-key"
}

// agentKeyDir is where the Agent pod mounts an SMB cell's key.
func agentKeyDir(c cell) string {
	return agentSecretsMountRoot + "/" + c.name
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

// stackNamePattern keeps a reused stack name valid as a Pulumi stack name once
// the framework prefixes it with the user name.
var stackNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// runOptions are the inputs of one run, read from the environment by
// TestAzureFiles.
type runOptions struct {
	runID       string
	writerImage string
	profile     string
	// cells is a comma-separated cell filter; empty selects every cell.
	cells string
	// stackName reuses one stack across runs. Empty gives each run its own.
	stackName string
	// smbEnabled provisions the smb cell, which needs an Agent built with the
	// native SMB log source.
	smbEnabled bool
}

type runSpec struct {
	runID       string
	stackName   string
	writerImage string
	profile     agentProfile
	// cells are provisioned and asserted on.
	cells []cell
	// gatedCells were selected but are left out of the stack because their
	// opt-in is not set. The test reports them as skipped.
	gatedCells []cell
}

func newRunSpec(opts runOptions) (runSpec, error) {
	profile, err := parseProfile(opts.profile)
	if err != nil {
		return runSpec{}, err
	}
	stackName, err := resolveStackName(opts.stackName, opts.runID)
	if err != nil {
		return runSpec{}, err
	}

	// Storage accounts follow the stack, so a reused stack keeps them. Shares
	// follow the run, so every run starts from empty shares: no ledger is left
	// over, and the SMB source's registry entries, which name the share, cannot
	// match an earlier run.
	stackDigest := hexDigest(stackName)
	runDigest := hexDigest(opts.runID)
	lineFingerprint := fingerprintConfig{strategy: "line_checksum", count: 1, maxBytes: 4096}
	all := []cell{
		{
			name:         "file-line",
			reader:       fileReader,
			fingerprint:  lineFingerprint,
			mountOptions: mountOptionsActimeo1,
			accountName:  "ddafline" + stackDigest[:14],
			shareName:    "fline-" + runDigest[:12],
		},
		{
			name:         "file-byte",
			reader:       fileReader,
			fingerprint:  fingerprintConfig{strategy: "byte_checksum", count: 2048, maxBytes: 2048},
			mountOptions: mountOptionsActimeo1,
			accountName:  "ddafbyte" + stackDigest[:14],
			shareName:    "fbyte-" + runDigest[:12],
		},
		{
			name:         "file-line-actimeo30",
			reader:       fileReader,
			fingerprint:  lineFingerprint,
			mountOptions: mountOptionsActimeo30,
			accountName:  "ddafln30" + stackDigest[:14],
			shareName:    "fline30-" + runDigest[:12],
		},
		{
			name:         "smb",
			reader:       smbReader,
			mountOptions: mountOptionsActimeo1,
			accountName:  "ddafsmb" + stackDigest[:14],
			shareName:    "smb-" + runDigest[:12],
		},
	}
	for i := range all {
		all[i].markers = markerDelaysFor(all[i].reader)
		all[i].service = "azure-files-" + all[i].name
		all[i].volumeName = "azure-files-" + all[i].name
		all[i].writerName = "writer-" + all[i].name
	}

	selected, err := filterCells(all, opts.cells)
	if err != nil {
		return runSpec{}, err
	}
	spec := runSpec{
		runID:       opts.runID,
		stackName:   stackName,
		writerImage: opts.writerImage,
		profile:     profile,
	}
	for _, c := range selected {
		if c.reader == smbReader && !opts.smbEnabled {
			spec.gatedCells = append(spec.gatedCells, c)
			continue
		}
		spec.cells = append(spec.cells, c)
	}
	return spec, nil
}

// resolveStackName returns the reused stack name when one is given, and a
// stack of this run's own otherwise.
func resolveStackName(override, runID string) (string, error) {
	override = strings.TrimSpace(override)
	if override == "" {
		return "azure-files-" + hexDigest(runID)[:8], nil
	}
	if !stackNamePattern.MatchString(override) {
		return "", fmt.Errorf("stack name %q must be 1 to 40 lowercase letters, digits or inner hyphens", override)
	}
	return override, nil
}

func hexDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
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

func (spec runSpec) hasReader(reader readerKind) bool {
	for _, c := range spec.cells {
		if c.reader == reader {
			return true
		}
	}
	return false
}

// fakeintakeOptions must be identical in both passes: the passes update one
// stack, and any difference would replace the Fakeintake VM between them.
func fakeintakeOptions() azurekubernetes.ProvisionerOption {
	return azurekubernetes.WithFakeIntakeOptions(
		fakeintake.WithRetentionPeriod(fakeintakeRetention),
		fakeintake.WithoutDDDevForwarding(),
	)
}

// storageProvisioner creates AKS and the Azure Files resources without an
// Agent. The second UpdateEnv pass installs the Agent only after the CSI
// secrets and shares already exist on the stack.
func (spec runSpec) storageProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	return azurekubernetes.AKSProvisioner(
		azurekubernetes.WithName("azurefiles"),
		fakeintakeOptions(),
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
		fakeintakeOptions(),
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
//
// The account keys only ever leave Azure as Pulumi secrets inside Kubernetes
// Secrets: one per cell in the e2e namespace for the CSI driver, and, for SMB
// cells, a copy in the Agent namespace that the Agent pod mounts as a file.
// Neither is a stack output.
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
	agentSecretOpts := append([]pulumi.ResourceOption{}, kubeOpts...)
	kubeOpts = append(kubeOpts, utils.PulumiDependsOn(namespace))

	if env.ImagePullRegistry() != "" {
		if _, err := utils.NewImagePullSecret(env, e2eNamespace, kubeOpts...); err != nil {
			return nil, err
		}
	}

	if spec.hasReader(smbReader) {
		// The Agent is installed by the second pass, but the key it mounts
		// must exist before its pod starts. A patch rather than a Namespace
		// leaves the namespace shared with the Agent installation, which
		// patches it too.
		agentNS, err := corev1.NewNamespacePatch(ctx, "azure-files-agent-namespace", &corev1.NamespacePatchArgs{
			Metadata: &metav1.ObjectMetaPatchArgs{
				Name: pulumi.String(agentNamespace),
			},
		}, agentSecretOpts...)
		if err != nil {
			return nil, err
		}
		agentSecretOpts = append(agentSecretOpts, utils.PulumiDependsOn(agentNS))
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
		rawAccountKey := keys.MapIndex(pulumi.String("keys")).ApplyT(firstStorageAccountKey).(pulumi.StringOutput)
		accountKey := pulumi.ToSecret(rawAccountKey).(pulumi.StringOutput)
		storageSecretOpts := append([]pulumi.ResourceOption{}, kubeOpts...)
		storageSecretOpts = append(storageSecretOpts, utils.PulumiDependsOn(share))
		_, err = corev1.NewSecret(ctx, c.secretName(), &corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(c.secretName()),
				Namespace: pulumi.String(e2eNamespace),
			},
			StringData: pulumi.StringMap{
				accountNameSecretKey: account.Name,
				accountKeySecretKey:  accountKey,
			},
		}, storageSecretOpts...)
		if err != nil {
			return nil, err
		}

		if c.reader == smbReader {
			// A Secret volume can only reference a Secret of the pod's own
			// namespace, so the SMB source's key is copied next to the Agent.
			_, err = corev1.NewSecret(ctx, c.secretName()+"-agent", &corev1.SecretArgs{
				Metadata: metav1.ObjectMetaArgs{
					Name:      pulumi.String(c.secretName()),
					Namespace: pulumi.String(agentNamespace),
					Labels: pulumi.StringMap{
						partOfLabel: pulumi.String(partOfLabelValue),
						cellLabel:   pulumi.String(c.name),
					},
				},
				StringData: pulumi.StringMap{
					accountKeySecretKey: accountKey,
				},
			}, agentSecretOpts...)
			if err != nil {
				return nil, err
			}
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
	runtime := spec.workloadRuntime(env.InternalDockerhubMirror())
	if len(runtime.scripts) > 0 {
		configMap, err := corev1.NewConfigMap(ctx, workloadConfigMapName, &corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(workloadConfigMapName),
				Namespace: pulumi.String(e2eNamespace),
				Labels: pulumi.StringMap{
					partOfLabel: pulumi.String(partOfLabelValue),
					runIDLabel:  pulumi.String(spec.runID),
				},
			},
			Data: pulumi.ToStringMap(runtime.scripts),
		}, kubeOpts...)
		if err != nil {
			return nil, err
		}
		kubeOpts = append(kubeOpts, utils.PulumiDependsOn(configMap))
	}
	for _, c := range spec.cells {
		if _, err := spec.newWriterDeployment(env, c, runtime, kubeOpts); err != nil {
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

// writerEnv is the writer configuration that both writers read.
func (spec runSpec) writerEnv(c cell) []envVar {
	return []envVar{
		{"LOGWRITER_LOG_DIR", logMountPath},
		{"LOGWRITER_RUN_ID", spec.writerRunID(c)},
		{"LOGWRITER_TARGET_BYTES_SEQUENCE", writerTargetSequence},
		{"LOGWRITER_HEAD_PAUSE_MS", "5000"},
		{"LOGWRITER_MAX_RECORDS_PER_PERIOD", "5000"},
		{"TZ", "UTC"},
	}
}

func toEnvVarArray(vars []envVar) corev1.EnvVarArray {
	array := make(corev1.EnvVarArray, 0, len(vars))
	for _, v := range vars {
		array = append(array, corev1.EnvVarArgs{Name: pulumi.String(v.name), Value: pulumi.String(v.value)})
	}
	return array
}

func (spec runSpec) newWriterDeployment(
	env config.Env,
	c cell,
	runtime workloadRuntime,
	baseOpts []pulumi.ResourceOption,
) (*appsv1.Deployment, error) {
	labels := pulumi.StringMap{
		"app.kubernetes.io/name": pulumi.String(c.writerName),
		partOfLabel:              pulumi.String(partOfLabelValue),
		runIDLabel:               pulumi.String(spec.runID),
		cellLabel:                pulumi.String(c.name),
	}
	templateMetadata := metav1.ObjectMetaArgs{Labels: labels}
	if len(runtime.scripts) > 0 {
		templateMetadata.Annotations = pulumi.StringMap{
			workloadChecksumAnnotation: pulumi.String(runtime.scriptsChecksum()),
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
				Metadata: templateMetadata,
				Spec:     spec.writerPodSpec(c, runtime, env.ImagePullRegistry() != ""),
			},
		},
	}, opts...)
}

// writerPodSpec runs a cell's writer, ledger and appender on its share.
func (spec runSpec) writerPodSpec(c cell, runtime workloadRuntime, imagePullSecret bool) *corev1.PodSpecArgs {
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
	volumes := corev1.VolumeArray{volume}
	volumeMounts := corev1.VolumeMountArray{volumeMount}
	if len(runtime.scripts) > 0 {
		volumes = append(volumes, corev1.VolumeArgs{
			Name: pulumi.String(workloadVolumeName),
			ConfigMap: corev1.ConfigMapVolumeSourceArgs{
				Name:        pulumi.String(workloadConfigMapName),
				DefaultMode: pulumi.Int(0o555),
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMountArgs{
			Name:      pulumi.String(workloadVolumeName),
			MountPath: pulumi.String(workloadDir),
			ReadOnly:  pulumi.Bool(true),
		})
	}

	writer := corev1.ContainerArgs{
		Name:            pulumi.String(writerContainerName),
		Image:           pulumi.String(runtime.image),
		ImagePullPolicy: pulumi.String("IfNotPresent"),
		Env:             toEnvVarArray(append(spec.writerEnv(c), runtime.writerEnv...)),
		VolumeMounts:    volumeMounts,
		ReadinessProbe: corev1.ProbeArgs{
			Exec: corev1.ExecActionArgs{Command: pulumi.ToStringArray([]string{
				"/bin/sh", "-c", fmt.Sprintf("test $(wc -c < %s/%s) -ge 4096", logMountPath, activeLogName),
			})},
			PeriodSeconds:    pulumi.Int(2),
			FailureThreshold: pulumi.Int(45),
		},
	}
	if runtime.writerCommand != nil {
		writer.Command = pulumi.ToStringArray(runtime.writerCommand)
	}
	podSpec := &corev1.PodSpecArgs{
		NodeSelector: pulumi.StringMap{
			"kubernetes.io/arch": pulumi.String("amd64"),
			"kubernetes.io/os":   pulumi.String("linux"),
		},
		Containers: corev1.ContainerArray{
			writer,
			corev1.ContainerArgs{
				Name:            pulumi.String(ledgerContainerName),
				Image:           pulumi.String(runtime.image),
				ImagePullPolicy: pulumi.String("IfNotPresent"),
				Command:         pulumi.ToStringArray(runtime.ledgerCommand),
				Env:             toEnvVarArray(append([]envVar{{"LOGWRITER_LOG_DIR", logMountPath}}, runtime.ledgerEnv...)),
				VolumeMounts:    volumeMounts,
			},
			// The appender shares the writer pod, so it appends through the
			// same CIFS mount that performed the rename. The rotation is
			// therefore visible to it immediately, whatever actimeo the cell
			// uses for revalidating remotely changed metadata.
			corev1.ContainerArgs{
				Name:            pulumi.String(appenderContainerName),
				Image:           pulumi.String(runtime.image),
				ImagePullPolicy: pulumi.String("IfNotPresent"),
				Command:         pulumi.ToStringArray(runtime.appenderCommand),
				Env: toEnvVarArray([]envVar{
					{"LOGWRITER_LOG_DIR", logMountPath},
					{"LOGWRITER_RUN_ID", spec.writerRunID(c)},
					{"LOGWRITER_MARKER_JOURNAL_PATH", logMountPath + "/" + postRotationMarkerJournalName},
					{"LOGWRITER_APPEND_DELAYS_MS", c.markers.appenderValue()},
					{"LOGWRITER_APPEND_POLL_MS", strconv.Itoa(postRotationMarkerPollMs)},
					{"TZ", "UTC"},
				}),
				VolumeMounts: volumeMounts,
			},
		},
		Volumes: volumes,
	}
	if runtime.kind == stockWorkload {
		podSpec.SecurityContext = corev1.PodSecurityContextArgs{
			RunAsNonRoot: pulumi.Bool(true),
			RunAsUser:    pulumi.Int(stockWorkloadUser),
			RunAsGroup:   pulumi.Int(stockWorkloadUser),
		}
	}
	if imagePullSecret {
		podSpec.ImagePullSecrets = corev1.LocalObjectReferenceArray{
			corev1.LocalObjectReferenceArgs{Name: pulumi.String(utils.DefaultImagePullSecretName)},
		}
	}
	return podSpec
}

// agentMountPath is where the Agent mounts the share of a file reader cell.
func agentMountPath(c cell) string {
	return logMountPath + "/" + c.name
}

// smbPasswordHandle is the secret backend handle the SMB source resolves to
// the cell's storage account key. The key reaches the Agent pod only as a file
// of its mounted Secret; /readsecret_multiple_providers.sh reads that file.
func smbPasswordHandle(c cell) string {
	return fmt.Sprintf("ENC[file@%s/%s]", agentKeyDir(c), accountKeySecretKey)
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
		case smbReader:
			// The writer writes at the share root, so the path is relative to
			// it. Rotated names do not match it: the source has to follow the
			// rotated file by its server FileId to drain it.
			fmt.Fprintf(&sources, `      - type: smb
        path: %s
        service: %s
        source: java
        start_position: beginning
        smb:
          host: %s
          share: %s
          username: %s
          password: %q
          poll_interval: %d
`, activeLogName, c.service, c.host(), c.shareName, c.accountName, smbPasswordHandle(c), smbPollIntervalSeconds)
			// The chart mounts agents.volumeMounts into every container of
			// the Agent pod; only the core Agent resolves the handle. The
			// Agent runs as root, so 0400 (256) still lets it read the key.
			fmt.Fprintf(&volumes, `    - name: %s
      secret:
        secretName: %s
        defaultMode: 256
        items:
          - key: %s
            path: %s
`, c.agentKeyVolumeName(), c.secretName(), accountKeySecretKey, accountKeySecretKey)
			fmt.Fprintf(&mounts, `    - name: %s
      mountPath: %s
      readOnly: true
`, c.agentKeyVolumeName(), agentKeyDir(c))
		}
		fmt.Fprintf(&sources, `        tags:
          - e2e_run_id:%s
          - e2e_cell:%s
          - e2e_profile:%s
`, spec.runID, c.name, spec.profile.name)
	}

	var values strings.Builder
	values.WriteString(`datadog:
  logs:
    enabled: true
    containerCollectAll: false
  logLevel: DEBUG
`)
	if spec.hasReader(smbReader) {
		// The handle names a mounted file, so the Agent needs no permission
		// to read Secrets through the API. Without
		// enableGlobalPermissions: false the chart would grant it every
		// Secret in the cluster.
		values.WriteString(`  secretBackend:
    command: /readsecret_multiple_providers.sh
    enableGlobalPermissions: false
`)
	}
	fmt.Fprintf(&values, `  confd:
    azure_files.yaml: |-
      logs:
%s`, sources.String())
	// The settings go to the core Agent container as a map. A datadog.env list
	// would replace the framework's own, which points the Agent at Fakeintake:
	// Helm replaces lists from later values files instead of merging them.
	fmt.Fprintf(&values, `agents:
  containers:
    agent:
      envDict:
        DD_LOGS_CONFIG_FILE_SCAN_PERIOD: "%d"
        DD_LOGS_CONFIG_CLOSE_TIMEOUT: "%d"
        DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED: "%t"
`, fileScanPeriodSeconds, closeTimeoutSeconds, spec.profile.unreliableMount)
	if volumes.Len() > 0 {
		fmt.Fprintf(&values, `  volumes:
%s  volumeMounts:
%s`, volumes.String(), mounts.String())
	}
	return values.String()
}
