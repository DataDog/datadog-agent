// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type agentWindowsManager struct {
	host *remoteComp.Host
}

func newWindowsManager(host *remoteComp.Host) agentOSManager {
	return &agentWindowsManager{host: host}
}

func (am *agentWindowsManager) directInstallCommand(env config.Env, packagePath string, version agentparams.PackageVersion, additionalInstallParameters []string, opts ...pulumi.ResourceOption) (command.Command, error) {
	cmd := fmt.Sprintf(`
$ProgressPreference = 'SilentlyContinue';
$ErrorActionPreference = 'Stop';
`)
	installCommandStr, err := am.getInstallPackageCommand(packagePath, version, additionalInstallParameters)
	if err != nil {
		return nil, err
	}
	cmd += installCommandStr
	return am.host.OS.Runner().Command("install-agent", &command.Args{Create: pulumi.Sprintf(cmd, env.AgentAPIKey())}, opts...)
}

func (am *agentWindowsManager) getInstallCommand(version agentparams.PackageVersion, apiKey pulumi.StringInput, additionalInstallParameters []string) (pulumi.StringOutput, error) {
	url, err := getAgentURL(version)
	if err != nil {
		return pulumi.Sprintf(""), err
	}

	cmd := ""
	if version.Flavor == agentparams.FIPSFlavor {
		cmd = fmt.Sprint(`
Set-ItemProperty -Path 'HKLM:\System\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy' -Name 'Enabled' -Value 1 -Type DWORD`)
	}

	localFilename := `C:\datadog-agent.msi`
	cmd += fmt.Sprintf(`
$ProgressPreference = 'SilentlyContinue';
$ErrorActionPreference = 'Stop';
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
for ($i=0; $i -lt 3; $i++) {
	try {
		(New-Object Net.WebClient).DownloadFile('%s','%s')
	} catch {
		if ($i -eq 2) {
			throw
		}
	}
};
`, url, localFilename)
	installPackageCommandStr, err := am.getInstallPackageCommand(localFilename, version, additionalInstallParameters)
	if err != nil {
		return pulumi.Sprintf(""), err
	}
	cmd += installPackageCommandStr

	return pulumi.Sprintf(cmd, apiKey), nil
}

func (am *agentWindowsManager) getInstallPackageCommand(filePath string, version agentparams.PackageVersion, additionalInstallParameters []string) (string, error) {
	logFilePath := "C:\\install.log"
	logParamIdx := slices.IndexFunc(additionalInstallParameters, func(s string) bool {
		return strings.HasPrefix(s, "/log")
	})
	if logParamIdx < 0 {
		additionalInstallParameters = append(additionalInstallParameters, fmt.Sprintf("/log %s", logFilePath))
	} else {
		// "/log C:\mycustomlog.txt" -> "C:\mycustomlog.txt"
		paramParts := strings.Split(additionalInstallParameters[logParamIdx], " ")
		if len(paramParts) != 2 {
			return "", fmt.Errorf("/log parameter was malformed, must be '/log <path_to_log_file>'")
		}
		logFilePath = paramParts[1]
	}
	cmd := ""
	if version.Flavor == agentparams.FIPSFlavor {
		cmd = fmt.Sprintf(`
Set-ItemProperty -Path 'HKLM:\System\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy' -Name 'Enabled' -Value 1 -Type DWORD`)
	}
	cmd += fmt.Sprintf(`
$exitCode = (Start-Process -Wait msiexec -PassThru -ArgumentList '/qn /i %s APIKEY=%%s %s').ExitCode
Get-Content %s
Exit $exitCode
	`, filePath, strings.Join(additionalInstallParameters, " "), logFilePath)
	return cmd, nil
}

func (am *agentWindowsManager) getAgentConfigFolder() string {
	return `C:\ProgramData\Datadog`
}

func (am *agentWindowsManager) restartAgentServices(transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error) {
	// TODO: When we introduce Namer in components, we should use it here.
	cmdName := am.host.Name() + "-" + "restart-agent"
	// Retry restart several time, workaround to https://datadoghq.atlassian.net/browse/WINA-747
	cmd := `
$tries = 0
$sleepTime = 1
while ($tries -lt 5) {
 & "$($env:ProgramFiles)\Datadog\Datadog Agent\bin\agent.exe" restart-service 2>>stderr.txt
 $exitCode = $LASTEXITCODE
 if ($exitCode -eq 0) {
	   break
 }
 Start-Sleep -Seconds $sleepTime
 $sleepTime = $sleepTime * 2
 $tries++
 }
 Get-Content stderr.txt
 Exit $exitCode
 `

	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Create: pulumi.String(cmd),
	}

	// If a transform is provided, use it to modify the command name and args
	if transform != nil {
		cmdName, cmdArgs = transform(cmdName, cmdArgs)
	}

	return am.host.OS.Runner().Command(cmdName, cmdArgs, opts...)
}

func (am *agentWindowsManager) ensureAgentUninstalled(version agentparams.PackageVersion, opts ...pulumi.ResourceOption) (command.Command, error) {
	uninstallCmd := `
$productCode = (@(Get-ChildItem -Path "HKLM:SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall" -Recurse) | Where {$_.GetValue("DisplayName") -like "Datadog Agent" }).PSChildName
if (!$productCode) {
    Write-Host "No Datadog Agent installation found to uninstall"
    exit 0
}
start-process msiexec -Wait -ArgumentList ('/log', 'C:\uninst.log', '/q', '/x', "$productCode", 'REBOOT=ReallySuppress')
`
	return am.host.OS.Runner().Command("uninstall-agent", &command.Args{Create: pulumi.String(uninstallCmd), Update: nil, Triggers: pulumi.Array{
		pulumi.String(version.Major),
		pulumi.String(version.Minor),
		pulumi.String(version.PipelineID),
		pulumi.String(version.Flavor),
		pulumi.String(version.Channel),
	}}, opts...)
}

func getAgentURL(version agentparams.PackageVersion) (string, error) {
	if version.Flavor == "" {
		version.Flavor = agentparams.DefaultFlavor
	}
	minor := strings.ReplaceAll(version.Minor, "~", "-")
	fullVersion := fmt.Sprintf("%v.%v", version.Major, minor)

	if version.PipelineID != "" {
		return getAgentURLFromPipelineID(version)
	}

	if version.Channel == agentparams.BetaChannel {
		finder, err := newAgentURLFinder("https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/installers_v2.json", version.Flavor)
		if err != nil {
			return "", err
		}

		url, err := finder.findVersion(fullVersion)
		if err != nil {
			return "", err
		}

		return url, nil
	}

	finder, err := newAgentURLFinder("https://ddagent-windows-stable.s3.amazonaws.com/installers_v2.json", version.Flavor)
	if err != nil {
		return "", err
	}

	if version.Minor == "" { // Use latest
		if fullVersion, err = finder.getLatestVersion(); err != nil {
			return "", err
		}
	} else {
		fullVersion += "-1"
	}

	return finder.findVersion(fullVersion)
}

func getAgentURLFromPipelineID(version agentparams.PackageVersion) (string, error) {
	url, err := getPipelineArtifact(version.PipelineID, "dd-agent-mstesting", version.Major, func(artifact string) bool {
		if !strings.Contains(artifact, fmt.Sprintf("%s-%s", version.Flavor, version.Major)) {
			return false
		}
		if !strings.HasSuffix(artifact, ".msi") {
			return false
		}

		return true
	})
	if err != nil {
		return "", err
	}

	return url, nil
}

// getPipelineArtifact searches a public S3 bucket for a given artifact from a Gitlab pipeline
// majorVersion = [6,7]
// predicate = A function taking the artifact name (from github.com/aws/aws-sdk-go-v2/service/s3/types.Object.Key)
// and that returns true when the artifact matches.
func getPipelineArtifact(pipelineID, bucket, majorVersion string, predicate func(string) bool) (string, error) {
	// TODO: Replace context.Background() with a Pulumi context.Context.
	// dd-agent-mstesting is a public bucket so we can use anonymous credentials
	config, err := awsConfig.LoadDefaultConfig(context.Background(), awsConfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
	if err != nil {
		return "", err
	}

	s3Client := s3.NewFromConfig(config)

	// Manual URL example: https://s3.amazonaws.com/dd-agent-mstesting?prefix=pipelines/A7/25309493
	result, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(fmt.Sprintf("pipelines/A%s/%s", majorVersion, pipelineID)),
	})

	if err != nil {
		return "", err
	}

	if len(result.Contents) <= 0 {
		return "", fmt.Errorf("no artifact found for pipeline %v", pipelineID)
	}

	for _, obj := range result.Contents {
		if !predicate(*obj.Key) {
			continue
		}

		return fmt.Sprintf("https://s3.amazonaws.com/%s/%s", bucket, *obj.Key), nil
	}

	return "", fmt.Errorf("no agent artifact found for pipeline %v", pipelineID)
}

type agentURLFinder struct {
	versions     map[string]interface{}
	installerURL string
}

func newAgentURLFinder(installerURL string, flavor string) (*agentURLFinder, error) {
	resp, err := http.Get(installerURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	values := make(map[string]interface{})
	if err = json.Unmarshal(body, &values); err != nil {
		return nil, err
	}

	versions, err := getKey[map[string]interface{}](values, flavor)
	if err != nil {
		return nil, err
	}
	return &agentURLFinder{versions: versions, installerURL: installerURL}, nil
}

func (f *agentURLFinder) getLatestVersion() (string, error) {
	var versions []string
	for version := range f.versions {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	if len(versions) == 0 {
		return "", errors.New("no version found")
	}
	return versions[len(versions)-1], nil
}

func (f *agentURLFinder) findVersion(fullVersion string) (string, error) {
	version, err := getKey[map[string]interface{}](f.versions, fullVersion)
	if err != nil {
		return "", fmt.Errorf("the Agent version %v cannot be found at %v: %v", fullVersion, f.installerURL, err)
	}

	arch, err := getKey[map[string]interface{}](version, "x86_64")
	if err != nil {
		return "", fmt.Errorf("cannot find `x86_64` for Agent version %v at %v: %v", fullVersion, f.installerURL, err)
	}

	url, err := getKey[string](arch, "url")
	if err != nil {
		return "", fmt.Errorf("cannot find `url` for Agent version %v at %v: %v", fullVersion, f.installerURL, err)
	}

	return url, nil
}

func getKey[T any](m map[string]interface{}, keyName string) (T, error) {
	var t T
	abstractValue, ok := m[keyName]
	if !ok {
		return t, fmt.Errorf("cannot find the key %v", keyName)
	}

	value, ok := abstractValue.(T)
	if !ok {
		return t, fmt.Errorf("%v doesn't have the right type: %v", keyName, reflect.TypeOf(t))
	}
	return value, nil
}
