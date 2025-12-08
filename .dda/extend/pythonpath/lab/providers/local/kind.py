# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Kind (Kubernetes in Docker) provider."""

from __future__ import annotations

import os
import shutil
import tempfile
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from lab.providers import BaseProvider, MissingPrerequisite, Option, ProviderConfig, ProviderOptions, register_provider

if TYPE_CHECKING:
    from dda.cli.application import Application


@dataclass
class KindOptions(ProviderOptions):
    """Typed options for Kind provider."""

    # Kind-specific options
    k8s_version: str = "v1.32.0"
    install_agent: bool = False
    agent_image: str = ""
    load_image: str = ""
    build_agent: bool = False
    with_process_agent: bool = False
    with_trace_agent: bool = False
    with_system_probe: bool = False
    with_security_agent: bool = False
    helm_values: str = ""
    devenv: str = ""
    force: bool = False
    # Credentials (resolved from config/env)
    api_key: str = ""
    app_key: str = ""
    # Internal state (set during create)
    _local_image: bool = False

    @classmethod
    def from_config(cls, config: ProviderConfig) -> KindOptions:
        """Create KindOptions from ProviderConfig."""
        return cls(
            name=config.name,
            k8s_version=config.options.get("k8s_version", "v1.32.0"),
            install_agent=config.options.get("install_agent", False),
            agent_image=config.options.get("agent_image", ""),
            load_image=config.options.get("load_image", ""),
            build_agent=config.options.get("build_agent", False),
            with_process_agent=config.options.get("with_process_agent", False),
            with_trace_agent=config.options.get("with_trace_agent", False),
            with_system_probe=config.options.get("with_system_probe", False),
            with_security_agent=config.options.get("with_security_agent", False),
            helm_values=config.options.get("helm_values", ""),
            devenv=config.options.get("devenv", ""),
            force=config.options.get("force", False),
            api_key=config.get_api_key() or "",
            app_key=config.get_app_key() or "",
        )

    @property
    def wants_agent(self) -> bool:
        """Check if agent installation is requested."""
        return self.install_agent or self.build_agent or bool(self.load_image)


@register_provider
class KindProvider(BaseProvider):
    """Provider for local Kind clusters."""

    name = "kind"
    category = "local"
    description = "Kind cluster (Kubernetes in Docker)"
    options_class = KindOptions

    create_options = [
        Option("--k8s-version", default="v1.32.0", help="Kubernetes version"),
        Option("--install-agent", is_flag=True, help="Install Datadog Agent"),
        Option("--agent-image", default="", help="Custom agent image"),
        Option("--load-image", default="", help="Load existing local docker image into cluster"),
        Option("--build-agent", is_flag=True, help="Build local agent image before deploying"),
        Option("--with-process-agent", is_flag=True, help="Include process-agent in local build"),
        Option("--with-trace-agent", is_flag=True, help="Include trace-agent in local build"),
        Option("--with-system-probe", is_flag=True, help="Include system-probe in local build"),
        Option("--with-security-agent", is_flag=True, help="Include security-agent in local build"),
        Option("--helm-values", default="", help="Path to custom Helm values.yaml file"),
        Option("--devenv", default="", help="Developer environment ID for building (see dda env dev)"),
        Option("--force", "-f", is_flag=True, help="Recreate if exists"),
    ]

    def check_prerequisites(self, app: Application, options: KindOptions) -> list[MissingPrerequisite]:
        missing: list[MissingPrerequisite] = []

        if not shutil.which("kind"):
            missing.append(
                MissingPrerequisite(
                    name="kind",
                    remediation="https://kind.sigs.k8s.io/docs/user/quick-start/#installation",
                )
            )
        if not shutil.which("helm"):
            missing.append(
                MissingPrerequisite(
                    name="helm",
                    remediation="https://helm.sh/docs/intro/install/",
                )
            )

        # Check dev environment is running if building locally
        if options.build_agent:
            if not self._is_devenv_running(options.devenv):
                env_id = options.devenv or "default"
                missing.append(
                    MissingPrerequisite(
                        name=f"Developer environment '{env_id}' not running",
                        remediation="dda env dev start" + (f" --id {options.devenv}" if options.devenv else ""),
                    )
                )

        return missing

    def create(self, app: Application, options: KindOptions) -> dict[str, Any] | None:
        from lab.agent import build_local_image
        from lab.kind import cluster_exists, create_cluster, delete_cluster, load_image, show_cluster_info

        name = options.name

        # Build local agent image if requested (do this before cluster operations)
        if options.build_agent:
            options.agent_image = build_local_image(
                app,
                tag=options.agent_image or "datadog/agent-dev:local",
                process_agent=options.with_process_agent,
                trace_agent=options.with_trace_agent,
                system_probe=options.with_system_probe,
                security_agent=options.with_security_agent,
                devenv=options.devenv,
            )

        existing = cluster_exists(name)
        if existing:
            if options.force:
                app.display_info(f"Deleting existing cluster '{name}'...")
                delete_cluster(app, name)
                existing = False
            elif options.wants_agent:
                # Cluster exists but user wants to install/update agent - that's fine
                app.display_info(f"Cluster '{name}' exists. Installing/updating agent...")
            else:
                app.abort(f"Cluster '{name}' exists. Use --force to recreate.")

        # Create cluster if needed
        if not existing:
            app.display_info(f"Creating kind cluster '{name}' with Kubernetes {options.k8s_version}...")

            kind_config = """
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
"""
            with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
                f.write(kind_config)
                config_path = f.name

            try:
                create_cluster(app, name, config_path, f"kindest/node:{options.k8s_version}")
            finally:
                os.unlink(config_path)

            app.display_success(f"Cluster '{name}' created")
            show_cluster_info(app, name)

        # Load built image into cluster
        if options.build_agent and options.agent_image:
            load_image(app, name, options.agent_image)
            options._local_image = True

        # Load existing image if specified
        if options.load_image:
            load_image(app, name, options.load_image)
            if not options.agent_image:
                options.agent_image = options.load_image
            options._local_image = True

        # Install agent if requested
        if options.wants_agent:
            self._install_agent(app, name, options)

        app.display_success(f"Cluster '{name}' is ready")
        app.display_info(f"Use: kubectl config use-context kind-{name}")

        # Return metadata about what was created
        return {
            "context": f"kind-{name}",
            "k8s_version": options.k8s_version,
            "agent_installed": options.wants_agent,
            "agent_image": options.agent_image or None,
        }

    def destroy(self, app: Application, name: str) -> None:
        from lab.kind import cluster_exists, delete_cluster

        if not cluster_exists(name):
            app.display_warning(f"Cluster '{name}' does not exist in kind")
            return

        app.display_info(f"Destroying cluster '{name}'...")
        delete_cluster(app, name)
        app.display_success(f"Cluster '{name}' destroyed")

    def _is_devenv_running(self, devenv: str) -> bool:
        """Check if the developer environment is running."""
        import subprocess

        env_id = devenv or "default"
        cmd = ["dda", "env", "dev", "status", "--id", env_id]

        result = subprocess.run(cmd, capture_output=True, text=True, check=False)
        return result.returncode == 0 and "started" in result.stdout.lower()

    def _install_agent(self, app: Application, name: str, options: KindOptions) -> None:
        from lab.agent import install_with_helm

        if not options.api_key:
            app.abort("API key required. Set E2E_API_KEY environment variable or configure in lab config.")

        # Use Never pull policy for local images (built or loaded)
        image_pull_policy = "Never" if options._local_image else "IfNotPresent"

        install_with_helm(
            app,
            cluster_name=name,
            api_key=options.api_key,
            app_key=options.app_key,
            agent_image=options.agent_image,
            helm_values=options.helm_values or None,
            image_pull_policy=image_pull_policy,
        )
