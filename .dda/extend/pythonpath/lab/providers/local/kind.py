# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Kind (Kubernetes in Docker) provider."""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any, cast

import click

from lab.capabilities import FakeintakeCapability
from lab.providers import BaseProvider, MissingPrerequisite, ProviderConfig, ProviderOptions, register_provider

if TYPE_CHECKING:
    from dda.cli.application import Application


@dataclass
class KindOptions(ProviderOptions):
    """Typed options for Kind provider."""

    # Kind-specific options
    k8s_version: str = "v1.32.0"
    no_agent: bool = False
    agent_image: str = ""
    load_image: str = ""
    build_agent: bool = False
    helm_values: str = ""
    devenv: str = ""
    force: bool = False
    fakeintake: bool = False
    update: bool = False
    # Credentials (resolved from config/env)
    api_key: str = ""
    app_key: str = ""
    build_command: str = ""
    # Internal state (set during create)
    _local_image: bool = False
    nodes_count: int = 2

    @classmethod
    def from_config(cls, config: ProviderConfig) -> KindOptions:
        """Create KindOptions from ProviderConfig."""
        return cls(
            name=config.name,
            k8s_version=config.options.get("k8s_version", "v1.32.0"),
            no_agent=config.options.get("no_agent", False),
            agent_image=config.options.get("agent_image", ""),
            load_image=config.options.get("load_image", ""),
            build_agent=config.options.get("build_agent", False),
            helm_values=config.options.get("helm_values", ""),
            devenv=config.options.get("devenv", ""),
            force=config.options.get("force", False),
            fakeintake=config.options.get("fakeintake", False),
            update=config.options.get("update", False),
            api_key=config.get_api_key() or "",
            app_key=config.get_app_key() or "",
            nodes_count=config.options.get("nodes_count", 2),
            build_command=config.options.get("build_command", ""),
        )

    @property
    def wants_agent(self) -> bool:
        """Check if agent installation is requested."""
        return not self.no_agent or self.build_agent or bool(self.load_image)


@register_provider
class KindProvider(BaseProvider, FakeintakeCapability):
    """Provider for local Kind clusters with fakeintake support."""

    name = "kind"
    category = "local"
    description = "Kind cluster (Kubernetes in Docker)"
    options_class = KindOptions

    # Fakeintake configuration constants
    FAKEINTAKE_NAMESPACE = "fakeintake"
    FAKEINTAKE_SERVICE = "fakeintake"
    FAKEINTAKE_LOCAL_PORT = 8080
    FAKEINTAKE_REMOTE_PORT = 80

    create_options = [
        click.option("--k8s-version", default="v1.32.0", help="Kubernetes version"),
        click.option("--no-agent", is_flag=True, default=False, help="Do not install the Datadog Agent"),
        click.option("--agent-image", default="", help="Custom agent image"),
        click.option("--load-image", default="", help="Load existing local docker image into cluster"),
        click.option("--helm-values", default="", help="Path to custom Helm values.yaml file"),
        click.option(
            "--build-command",
            default="",
            help="Command to build the agent image, output must be an image tagged with'datadog/agent-dev:local'",
        ),
        click.option("--devenv", default="", help="Developer environment ID for building (see dda env dev)"),
        click.option("--force", "-f", is_flag=True, help="Recreate if exists"),
        click.option("--nodes-count", default=2, help="Number of nodes in the cluster"),
        click.option("--fakeintake", is_flag=True, default=False, help="Deploy fakeintake for local testing"),
        click.option(
            "--update", "-u", is_flag=True, default=False, help="Update agent in existing cluster (much faster)"
        ),
    ]

    def check_prerequisites(self, app: Application, opts: ProviderOptions) -> list[MissingPrerequisite]:
        # Keep the BaseProvider signature (ProviderOptions) and cast locally to our typed options.
        options = cast(KindOptions, opts)
        missing: list[MissingPrerequisite] = []

        retcode = app.subprocess.attach(
            ["docker", "ps"], stderr=subprocess.DEVNULL, stdout=subprocess.DEVNULL, check=False
        ).returncode
        if retcode != 0:
            missing.append(
                MissingPrerequisite(
                    name="Docker installed and running",
                    remediation="docker ps failed, please check if Docker is installed and running",
                    actions={"create", "delete"},
                )
            )

        # Check if kind is installed
        if not shutil.which("kind"):
            missing.append(
                MissingPrerequisite(
                    name="kind",
                    remediation="https://kind.sigs.k8s.io/docs/user/quick-start/#installation",
                    actions={"create", "delete"},
                )
            )

        # Helm only needed when we're installing the Agent
        if options.wants_agent and not shutil.which("helm"):
            missing.append(
                MissingPrerequisite(
                    name="helm",
                    remediation="https://helm.sh/docs/intro/install/",
                    actions={"create"},
                )
            )

        # A dev environment is only needed if we're going to build a local image.
        if options.wants_agent and not options.agent_image and not options.load_image:
            if not self._is_devenv_running(options.devenv):
                env_id = options.devenv or "default"
                missing.append(
                    MissingPrerequisite(
                        name=f"Developer environment '{env_id}' not running",
                        remediation="dda env dev start" + (f" --id {options.devenv}" if options.devenv else ""),
                        actions={"create"},
                    )
                )

        return missing

    def create(self, app: Application, opts: ProviderOptions) -> dict[str, Any] | None:
        # Keep the BaseProvider signature (ProviderOptions) and cast locally to our typed options.
        options = cast(KindOptions, opts)
        from lab.agent import build_local_image
        from lab.kind import cluster_exists, create_cluster, delete_cluster, load_image, show_cluster_info

        name = options.name
        fakeintake_url = None

        # Ensure fakeintake image is available (pull from Docker Hub if needed)
        if options.fakeintake:
            from lab.fakeintake import ensure_fakeintake_image

            ensure_fakeintake_image(app)

        # Build local agent image if we are going to install the agent and no image was provided.
        # (Do this before cluster operations.)
        if options.wants_agent and not options.agent_image and not options.load_image:
            options.agent_image = build_local_image(
                app, tag="datadog/agent-dev:local", devenv=options.devenv, build_command=options.build_command
            )
            options._local_image = True

        existing = cluster_exists(app, name)
        if existing:
            if options.update:
                # Fast update path: skip cluster creation, fakeintake deployment
                app.display_info(f"Cluster '{name}' exists. Updating agent only (fast mode)...")
                existing = True
            elif options.force:
                app.display_info(f"Deleting existing cluster '{name}'...")
                delete_cluster(app, name)
                existing = False
            elif not options.no_agent:
                # Cluster exists but user wants to install/update agent - that's fine
                app.display_info(f"Cluster '{name}' exists. Installing/updating agent...")
            else:
                app.abort(f"Cluster '{name}' exists. Use --force to recreate or --update to update agent.")

        # Create cluster if needed
        if not existing:
            if options.nodes_count <= 0:
                app.abort("Number of nodes must be strictly greater than 0")

            app.display_info(f"Creating kind cluster '{name}' with Kubernetes {options.k8s_version}...")

            kind_config = """
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
"""
            kind_config += "\n".join(["- role: worker" for _ in range(options.nodes_count - 1)])
            with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
                f.write(kind_config)
                config_path = f.name

            try:
                create_cluster(app, name, config_path, f"kindest/node:{options.k8s_version}")
            finally:
                os.unlink(config_path)

            app.display_success(f"Cluster '{name}' created")
            show_cluster_info(app, name)

        # Deploy fakeintake if requested (skip if updating)
        if options.fakeintake and not (options.update and existing):
            from lab.fakeintake import deploy_fakeintake

            fakeintake_url = deploy_fakeintake(app, name)
        elif options.fakeintake and options.update:
            # Fakeintake already deployed, just use existing URL
            fakeintake_url = "http://fakeintake.fakeintake.svc.cluster.local"
            app.display_info("Using existing fakeintake deployment")

        # Load built image into cluster
        if options.agent_image and options._local_image:
            load_image(app, name, options.agent_image)

        # Load existing image if specified
        if options.load_image:
            load_image(app, name, options.load_image)
            if not options.agent_image:
                options.agent_image = options.load_image
            options._local_image = True

        # Install agent if requested
        if options.wants_agent:
            self._install_agent(app, name, options, fakeintake_url)

        app.display_success(f"Cluster '{name}' is ready")
        app.display_info(f"Use: kubectl config use-context kind-{name}")

        if options.fakeintake:
            app.display_info("\n📊 Fakeintake is ready! Query metrics with:")
            app.display_info(f"  dda lab query --id {name} --list")
            app.display_info(f"  dda lab query --id {name} --metric system.cpu.idle")

        # Return metadata about what was created
        return {
            "context": f"kind-{name}",
            "k8s_version": options.k8s_version,
            "agent_installed": options.wants_agent,
            "agent_image": options.agent_image or None,
            "fakeintake_enabled": options.fakeintake,
            "fakeintake_url": fakeintake_url if options.fakeintake else None,
        }

    def destroy(self, app: Application, name: str) -> None:
        from lab.kind import cluster_exists, delete_cluster

        if not cluster_exists(app, name):
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

    def _install_agent(
        self, app: Application, name: str, options: KindOptions, fakeintake_url: str | None = None
    ) -> None:
        from lab.agent import install_with_helm

        # When using fakeintake, API key is not required (can be anything)
        api_key = options.api_key
        if not api_key:
            if options.fakeintake:
                api_key = "fakeintake"  # Dummy API key for fakeintake
            else:
                app.abort("API key required. Set E2E_API_KEY environment variable or configure in lab config.")

        # Use Never pull policy for local images (built or loaded)
        image_pull_policy = "Never" if options._local_image else "IfNotPresent"

        install_with_helm(
            app,
            cluster_name=name,
            api_key=api_key,
            app_key=options.app_key,
            agent_image=options.agent_image,
            helm_values=options.helm_values or None,
            image_pull_policy=image_pull_policy,
            fakeintake_url=fakeintake_url,
        )

    # Fakeintake Capability Implementation

    def _get_fakeintake_port_forward(self, app: Application, name: str) -> int:
        """
        Get or create port-forward for fakeintake service.

        Args:
            app: Application instance
            name: Environment name

        Returns:
            Local port number
        """
        from lab.port_forward import get_port_forward_manager

        pf_manager = get_port_forward_manager()
        return pf_manager.get_port_forward(
            app=app,
            env_name=name,
            service=self.FAKEINTAKE_SERVICE,
            namespace=self.FAKEINTAKE_NAMESPACE,
            local_port=self.FAKEINTAKE_LOCAL_PORT,
            remote_port=self.FAKEINTAKE_REMOTE_PORT,
            context=f"kind-{name}",
        )

    def query_fakeintake(
        self,
        app: Application,
        name: str,
        *,
        metric_name: str | None = None,
        list_metrics: bool = False,
        output_format: str = "pretty",
    ) -> str:
        """Query fakeintake for metrics."""
        from lab.fakeintake_client import ensure_client_built, query_metrics

        # Ensure client is built
        ensure_client_built(app)

        # Get or create port-forward
        local_port = self._get_fakeintake_port_forward(app, name)

        # Query fakeintake
        fakeintake_url = f"http://localhost:{local_port}"
        return query_metrics(
            fakeintake_url=fakeintake_url,
            metric_name=metric_name,
            list_metrics=list_metrics,
            output_format=output_format,
        )

    def get_fakeintake_url(self, app: Application, name: str) -> str:
        """Get the URL to access fakeintake (with automatic port-forwarding)."""
        local_port = self._get_fakeintake_port_forward(app, name)
        return f"http://localhost:{local_port}"
