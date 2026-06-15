// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command ai-sandbox provisions a Datadog-agent test environment (an AWS host with the
// agent installed) using the e2e provisioning framework, runs an AI coding agent
// (claude or codex) on the VM with a given model/effort/prompt, and retrieves a
// directory from the VM at the end.
//
// It is a standalone (non-test) consumer of the e2e framework, made possible by the
// decoupling of the client layer from *testing.T (see PR #51954).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

type options struct {
	// Provisioning
	stackName    string
	osDescriptor string
	arch         string
	instanceType string
	agentVersion string
	agentConfig  string
	noFakeintake bool
	keep         bool

	// AI agent
	tool       string
	model      string
	effort     string
	prompt     string
	promptFile string
	installCmd string
	toolArgs   string

	// Retrieval / output
	remoteOutputDir string
	localOutputDir  string
}

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func parseFlags(args []string) (*options, error) {
	fs := flag.NewFlagSet("ai-sandbox", flag.ContinueOnError)
	opts := &options{}

	// Provisioning
	fs.StringVar(&opts.stackName, "stack-name", "ai-sandbox", "Pulumi stack name to provision")
	fs.StringVar(&opts.osDescriptor, "os", "ubuntu:22-04", "OS descriptor (flavor:version, e.g. ubuntu:22-04, amazon-linux:2023, debian:12)")
	fs.StringVar(&opts.arch, "arch", "x86_64", "CPU architecture: x86_64 or arm64")
	fs.StringVar(&opts.instanceType, "instance-type", "", "EC2 instance type (empty uses the framework default)")
	fs.StringVar(&opts.agentVersion, "agent-version", "", "Agent version to install (empty installs the latest)")
	fs.StringVar(&opts.agentConfig, "agent-config", "", "datadog.yaml content (inline YAML) to apply on the agent")
	fs.BoolVar(&opts.noFakeintake, "no-fakeintake", false, "Do not provision a fakeintake")
	fs.BoolVar(&opts.keep, "keep", false, "Keep the stack after the run (skip teardown)")

	// AI agent
	fs.StringVar(&opts.tool, "tool", "claude", "AI agent to run on the VM: claude or codex")
	fs.StringVar(&opts.model, "model", "", "Model to use (passed to the tool's --model flag)")
	fs.StringVar(&opts.effort, "effort", "", "Reasoning effort (codex: model_reasoning_effort; ignored by claude)")
	fs.StringVar(&opts.prompt, "prompt", "", "Prompt to send to the AI agent")
	fs.StringVar(&opts.promptFile, "prompt-file", "", "Path to a local file whose content is used as the prompt")
	fs.StringVar(&opts.installCmd, "install-cmd", "", "Override the shell command used to install the tool on the VM")
	fs.StringVar(&opts.toolArgs, "tool-args", "", "Extra arguments appended to the tool invocation")

	// Retrieval / output
	fs.StringVar(&opts.remoteOutputDir, "remote-output-dir", "/tmp/ai-sandbox-output", "Directory on the VM to run the tool in and to retrieve afterwards")
	fs.StringVar(&opts.localOutputDir, "local-output-dir", "./ai-sandbox-output", "Local directory to download the remote output directory into")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if opts.tool != "claude" && opts.tool != "codex" {
		return nil, fmt.Errorf("unknown --tool %q (expected claude or codex)", opts.tool)
	}
	if opts.prompt == "" && opts.promptFile == "" {
		return nil, errors.New("one of --prompt or --prompt-file is required")
	}
	if opts.prompt != "" && opts.promptFile != "" {
		return nil, errors.New("--prompt and --prompt-file are mutually exclusive")
	}
	return opts, nil
}

func run(opts *options) error {
	prompt, err := resolvePrompt(opts)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(opts.localOutputDir, 0o755); err != nil {
		return fmt.Errorf("unable to create local output dir: %w", err)
	}

	ctx := standalone.NewContext(opts.localOutputDir)

	provisioner, err := buildProvisioner(opts)
	if err != nil {
		return err
	}

	// Register teardown before provisioning so partial resources are cleaned up even
	// if provisioning itself fails (the Pulumi provisioner does not delete on failure).
	if !opts.keep {
		defer func() {
			ctx.Logf("Destroying stack %q...", opts.stackName)
			if derr := standalone.Destroy(ctx, opts.stackName, provisioner); derr != nil {
				ctx.Logf("WARNING: failed to destroy stack %q: %v", opts.stackName, derr)
			}
		}()
	} else {
		ctx.Logf("--keep set: stack %q will be left running; destroy it manually when done", opts.stackName)
	}

	ctx.Logf("Provisioning stack %q (os=%s arch=%s)...", opts.stackName, opts.osDescriptor, opts.arch)
	env, err := standalone.Provision[environments.Host](ctx, opts.stackName, provisioner)
	if err != nil {
		return err
	}

	if env.RemoteHost == nil {
		return errors.New("provisioned environment has no RemoteHost")
	}

	runErr := runAITool(ctx, env, opts, prompt)

	// Always attempt retrieval, even when the tool failed, so tool-output.log and any
	// partial artifacts are available locally for inspection.
	ctx.Logf("Retrieving %s -> %s ...", opts.remoteOutputDir, opts.localOutputDir)
	getErr := env.RemoteHost.GetFolder(opts.remoteOutputDir, opts.localOutputDir)

	if runErr != nil {
		if getErr != nil {
			ctx.Logf("WARNING: failed to retrieve remote output dir: %v", getErr)
		}
		return runErr
	}
	if getErr != nil {
		return fmt.Errorf("failed to retrieve remote output dir: %w", getErr)
	}

	ctx.Logf("Done. Output available at %s", opts.localOutputDir)
	return nil
}

func resolvePrompt(opts *options) (string, error) {
	if opts.promptFile != "" {
		b, err := os.ReadFile(opts.promptFile)
		if err != nil {
			return "", fmt.Errorf("unable to read --prompt-file: %w", err)
		}
		return string(b), nil
	}
	return opts.prompt, nil
}

func buildProvisioner(opts *options) (provisioners.Provisioner, error) {
	desc, err := parseOSDescriptor(opts.osDescriptor, opts.arch)
	if err != nil {
		return nil, err
	}

	// desc already carries the architecture, validated in parseOSDescriptor.
	vmOpts := []ec2.VMOption{ec2.WithOS(desc)}
	if opts.instanceType != "" {
		vmOpts = append(vmOpts, ec2.WithInstanceType(opts.instanceType))
	}

	agentOpts := []agentparams.Option{}
	if opts.agentVersion != "" {
		agentOpts = append(agentOpts, agentparams.WithVersion(opts.agentVersion))
	}
	if opts.agentConfig != "" {
		agentOpts = append(agentOpts, agentparams.WithAgentConfig(opts.agentConfig))
	}

	runOpts := []ec2.Option{
		ec2.WithEC2InstanceOptions(vmOpts...),
		ec2.WithAgentOptions(agentOpts...),
	}
	if opts.noFakeintake {
		runOpts = append(runOpts, ec2.WithoutFakeIntake())
	}

	return awshost.Provisioner(awshost.WithRunOptions(runOpts...)), nil
}

func parseOSDescriptor(descStr, arch string) (desc oscomp.Descriptor, err error) {
	// oscomp.DescriptorFromString panics on malformed input; recover into an error.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid --os %q (expected flavor:version, e.g. ubuntu:22-04): %v", descStr, r)
		}
	}()
	desc = oscomp.DescriptorFromString(descStr, oscomp.UbuntuDefault)
	desc = desc.WithArch(oscomp.ArchitectureFromString(arch))
	return desc, nil
}

func runAITool(ctx *standalone.Context, env *environments.Host, opts *options, prompt string) error {
	host := env.RemoteHost

	// 1. Create the remote output directory (SFTP, not a shell command).
	if err := host.MkdirAll(opts.remoteOutputDir); err != nil {
		return fmt.Errorf("failed to create remote output dir: %w", err)
	}

	// 2. Upload the prompt. WriteFile transfers content over SFTP and logs only the
	//    path, so no file content ends up in the command log.
	if _, err := host.WriteFile(opts.remoteOutputDir+"/prompt.txt", []byte(prompt)); err != nil {
		return fmt.Errorf("failed to upload prompt: %w", err)
	}

	// 3. Write the credentials to a file OUTSIDE the retrieved directory, again via
	//    SFTP so the secret never appears in a logged command. The run script sources
	//    it and deletes it before running the tool.
	credsPath := opts.remoteOutputDir + ".creds.env"
	credsContent := buildCredsFile(opts.tool)
	hasCreds := credsContent != ""
	if hasCreds {
		if _, err := host.WriteFile(credsPath, []byte(credsContent)); err != nil {
			return fmt.Errorf("failed to upload credentials: %w", err)
		}
		// Restrict permissions; the path (not the content) is all that gets logged.
		if _, err := host.Execute(fmt.Sprintf("chmod 600 %s", shellQuote(credsPath))); err != nil {
			return fmt.Errorf("failed to secure credentials file: %w", err)
		}
		// Backstop: ensure the creds file is gone even if the run script exits before
		// its own cleanup (the script removes it itself on the normal path).
		defer func() { _, _ = host.Execute(fmt.Sprintf("rm -f %s", shellQuote(credsPath))) }()
	}

	// 4. Upload the run script.
	runScript, err := buildRunScript(opts, credsPath, hasCreds)
	if err != nil {
		return err
	}
	scriptPath := opts.remoteOutputDir + "/run.sh"
	if _, err := host.WriteFile(scriptPath, []byte(runScript)); err != nil {
		return fmt.Errorf("failed to upload run script: %w", err)
	}

	// 5. Install the tool.
	installCmd := opts.installCmd
	if installCmd == "" {
		installCmd = defaultInstallCmd(opts.tool)
	}
	ctx.Logf("Installing %s on the VM...", opts.tool)
	if out, err := host.Execute(installCmd); err != nil {
		return fmt.Errorf("failed to install %s: %w\n%s", opts.tool, err, out)
	}

	// 6. Run the tool. Credentials are sourced from the creds file inside the script,
	//    so the executed command carries no secret.
	ctx.Logf("Running %s (model=%q effort=%q)...", opts.tool, opts.model, opts.effort)
	out, err := host.Execute(fmt.Sprintf("bash %s", shellQuote(scriptPath)))
	// The tool's own stdout/stderr is captured to tool-output.log on the VM and
	// retrieved with the output dir; surface the wrapper output for quick feedback.
	if out != "" {
		ctx.Logf("%s output:\n%s", opts.tool, out)
	}
	if err != nil {
		return fmt.Errorf("%s run failed (see tool-output.log in the retrieved dir): %w", opts.tool, err)
	}
	return nil
}

// buildCredsFile returns shell `export` lines for whichever auth credentials the chosen
// tool can use and that are present in the local environment, or "" if none are set.
// Claude accepts an API key (ANTHROPIC_API_KEY) or a Claude Code OAuth token
// (CLAUDE_CODE_OAUTH_TOKEN); codex uses OPENAI_API_KEY.
func buildCredsFile(tool string) string {
	var names []string
	switch tool {
	case "claude":
		names = []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"}
	case "codex":
		names = []string{"OPENAI_API_KEY"}
	}

	var b strings.Builder
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			fmt.Fprintf(&b, "export %s=%s\n", name, shellQuote(v))
		}
	}
	return b.String()
}

func defaultInstallCmd(tool string) string {
	switch tool {
	case "claude":
		// Native installer; installs to ~/.local/bin.
		return "curl -fsSL https://claude.ai/install.sh | bash"
	case "codex":
		// Bootstrap Node.js/npm if missing (default Ubuntu host has neither), then
		// install the codex CLI globally. Targets Debian/Ubuntu; for other distros
		// pass --install-cmd. The global bin lands on PATH (/usr/bin).
		return "command -v npm >/dev/null 2>&1 || { curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -; sudo apt-get install -y nodejs; }; sudo npm install -g @openai/codex"
	default:
		return ""
	}
}

func buildRunScript(opts *options, credsPath string, hasCreds bool) (string, error) {
	var toolCmd string
	switch opts.tool {
	case "claude":
		toolCmd = "claude --permission-mode bypassPermissions"
		if opts.model != "" {
			toolCmd += " --model " + shellQuote(opts.model)
		}
		if opts.toolArgs != "" {
			toolCmd += " " + opts.toolArgs
		}
		toolCmd += ` -p "$(cat prompt.txt)"`
	case "codex":
		toolCmd = "codex exec --skip-git-repo-check"
		if opts.model != "" {
			toolCmd += " --model " + shellQuote(opts.model)
		}
		if opts.effort != "" {
			toolCmd += " -c model_reasoning_effort=" + shellQuote(opts.effort)
		}
		if opts.toolArgs != "" {
			toolCmd += " " + opts.toolArgs
		}
		toolCmd += ` "$(cat prompt.txt)"`
	default:
		return "", fmt.Errorf("unknown tool %q", opts.tool)
	}

	// Source the credentials file (if any) and remove it immediately so the secret is
	// not left on disk and is never part of the retrieved directory.
	credsSetup := ""
	if hasCreds {
		credsSetup = fmt.Sprintf("set -a; . %s; set +a; rm -f %s\n", shellQuote(credsPath), shellQuote(credsPath))
	}

	// Run inside the remote output dir (so files the agent writes land there) with the
	// tool install locations on PATH, capturing all output to tool-output.log.
	script := fmt.Sprintf(`#!/usr/bin/env bash
export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$HOME/bin:/usr/local/bin:$PATH"
%scd %s || exit 1
%s > tool-output.log 2>&1
`, credsSetup, shellQuote(opts.remoteOutputDir), toolCmd)
	return script, nil
}

// shellQuote single-quotes a string for safe use in a POSIX shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
