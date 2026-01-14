from __future__ import annotations

import hashlib
import json
import os
import shlex
import tempfile
import time
from pathlib import Path

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.junit_upload_core import get_datadog_ci_command
from tasks.libs.common.utils import gitlab_section

_vt_api_call_count = 0


def _increment_vt_call_count():
    """Increment the VT API call counter."""
    global _vt_api_call_count
    _vt_api_call_count += 1


def _sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def _find_artifact(omnibus_pipeline_dir: str, pattern: str) -> Path:
    base = Path(omnibus_pipeline_dir)
    matches = list(base.rglob(pattern))
    if not matches:
        raise Exception(f"No MSI file found for pattern {pattern} in {omnibus_pipeline_dir}")
    return matches[0]


def _ensure_vt_api_key(ctx) -> str:
    api_key = os.environ.get("VT_API_KEY", "").strip()
    if api_key:
        return api_key
    if os.path.exists(os.path.expanduser("~/.vt.toml")):
        return ""
    secret_ref = os.environ.get("VIRUS_TOTAL", "").strip()
    fetch_script = Path("tools/ci/fetch_secret.sh")
    if not api_key and secret_ref and fetch_script.exists():
        # Run the fetch script used in CI to retrieve the key
        res = ctx.run(f"{fetch_script} {secret_ref} api-key", hide=True, warn=True)
        if res.ok:
            api_key = res.stdout.strip()
    if not api_key:
        raise Exception("VT_API_KEY not set and could not be fetched. Set VT_API_KEY or VIRUS_TOTAL.")
    os.environ["VT_API_KEY"] = api_key
    return api_key


async def _fetch_file_report(apikey: str, file_sha256: str) -> dict:
    """
    Fetch file report from VirusTotal by SHA256 hash.
    File object attributes: https://docs.virustotal.com/reference/files
    """
    import vt  # Lazily import because virustotal dep is optional in CI

    async with vt.Client(apikey) as client:
        try:
            _increment_vt_call_count()
            file_obj = await client.get_object_async(f"/files/{file_sha256}")
            return file_obj.to_dict()
        except Exception as e:
            raise Exception(f"Error fetching file report: {e}") from e


async def _submit_scan(
    apikey: str,
    file_path: Path,
) -> str:
    """Submit a file for VirusTotal scanning and return the analysis ID."""

    import vt

    async with vt.Client(apikey) as client:
        try:
            with open(file_path, "rb") as f:
                _increment_vt_call_count()
                analysis = await client.scan_file_async(f, wait_for_completion=False)
            analysis_id = analysis.id
            return analysis_id
        except Exception as e:
            raise Exception(f"Failed to submit file for scan: {e}") from e


async def _poll_analysis(apikey: str, analysis_id: str, poll_timeout: int = 600, poll_interval: int = 30):
    """Poll analysis results until completion and return malicious/suspicious counts."""

    import vt

    async with vt.Client(apikey) as client:
        start_time = time.time()
        while True:
            try:
                _increment_vt_call_count()
                analysis = await client.get_object_async(f"/analyses/{analysis_id}")
                attributes = analysis.to_dict().get("attributes", {})
                status = attributes.get("status", "unknown")
                print(f"Analysis status: {status}")

                if status == "completed":
                    break

                if time.time() - start_time > poll_timeout:
                    raise Exception(f"Polling analysis timed out after {poll_timeout} seconds")

                time.sleep(poll_interval)
            except Exception as e:
                raise Exception(f"Error polling analysis: {e}") from e


def _format_engine_findings(file_report: dict) -> tuple[list[str], list[str]]:
    """Format engine findings from VT file report into malicious and suspicious lists."""
    attributes = file_report.get("attributes", file_report)
    findings = attributes.get("last_analysis_results", {}) or {}

    malicious = []
    suspicious = []
    for _, v in findings.items():
        if v.get("category") == "malicious":
            engine = v.get("engine_name", "unknown")
            version = v.get("engine_version") or "N/A"
            update = v.get("engine_update") or ""
            result = v.get("result", "")
            malicious.append(f"{engine} ({version} - {update}) -> malicious: {result}")
        elif v.get("category") == "suspicious":
            engine = v.get("engine_name", "unknown")
            version = v.get("engine_version") or "N/A"
            update = v.get("engine_update") or ""
            result = v.get("result", "")
            suspicious.append(f"{engine} ({version} - {update}) -> suspicious: {result}")
    return malicious, suspicious


def _write_junit_xml(
    output_path: Path,
    file_name: str,
    file_sha256: str,
    file_report: dict,
):
    import xml.etree.ElementTree as ET

    malicious_matches, suspicious_matches = _format_engine_findings(file_report)
    malicious = len(malicious_matches)
    suspicious = len(suspicious_matches)

    tests = 1
    failures = 1 if (malicious > 0 or suspicious > 0) else 0

    suites = ET.Element("testsuites")
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    suite = ET.SubElement(
        suites,
        "testsuite",
        attrib={
            "name": "VirusTotal Scan",
            "tests": str(tests),
            "failures": str(failures),
            "time": "0",
            "timestamp": ts,
        },
    )

    # Tags via properties per Datadog docs: dd_tags[key]
    props = ET.SubElement(suite, "properties")

    def add_tag(k: str, v: str):
        if v is None:
            v = ""
        ET.SubElement(props, "property", attrib={"name": f"dd_tags[{k}]", "value": str(v)})

    meta = file_report or {}
    attributes = meta.get("attributes", meta)

    add_tag("file.name", file_name)
    add_tag("file.sha256", file_sha256)
    add_tag(
        "vt.signature_verified",
        str(attributes.get("signature_info", {}).get("verified", "")),
    )
    add_tag("vt.times_submitted", str(attributes.get("times_submitted", "")))
    add_tag(
        "vt.last_modification_date",
        str(attributes.get("last_modification_date", "")),
    )
    add_tag(
        "vt.last_submission_date",
        str(attributes.get("last_submission_date", "")),
    )
    add_tag("vt.malicious", str(malicious))
    add_tag("vt.suspicious", str(suspicious))
    add_tag("vt.api_call_count", str(_vt_api_call_count))
    add_tag("vt.url", f"https://www.virustotal.com/gui/file/{file_sha256}")
    # Include GitLab CI metadata as tags
    add_tag("ci.pipeline.id", os.environ.get("CI_PIPELINE_ID", ""))
    add_tag("ci.pipeline.name", os.environ.get("CI_PIPELINE_NAME", ""))
    add_tag("ci.pipeline.url", os.environ.get("CI_PIPELINE_URL", ""))
    add_tag("ci.job.id", os.environ.get("CI_JOB_ID", ""))
    add_tag("ci.job.name", os.environ.get("CI_JOB_NAME", ""))
    add_tag("ci.job.url", os.environ.get("CI_JOB_URL", ""))
    add_tag("ci.project.url", os.environ.get("CI_PROJECT_URL", ""))

    case = ET.SubElement(
        suite,
        "testcase",
        attrib={
            "classname": "virustotal",
            "name": f"scan:{file_name}",
            "time": "0",
        },
    )

    if failures:
        msg_lines = (
            [
                f"Detections: malicious={malicious} suspicious={suspicious}",
                "",
                "Malicious engines:",
            ]
            + malicious_matches
            + [
                "",
                "Suspicious engines:",
            ]
            + suspicious_matches
        )
        failure = ET.SubElement(case, "failure", attrib={"message": "VirusTotal detections found"})
        failure.text = "\n".join(msg_lines)

    system_out = ET.SubElement(case, "system-out")
    system_out.text = json.dumps(file_report or {})[:100000]

    tree = ET.ElementTree(suites)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    tree.write(output_path, encoding="UTF-8", xml_declaration=True)

    return malicious, suspicious


@task(
    help={
        "omnibus_pipeline_dir": "Directory containing built artifacts (OMNIBUS_PIPELINE_DIR)",
        "pattern": "Glob pattern to find the MSI (e.g., datadog-agent-7*.msi)",
        "output": "Path to write JUnit XML (default: vt-junit.xml)",
        "upload": "If true, upload the JUnit to Datadog via datadog-ci",
        "sha256": "If set, fetch existing VT results by SHA256 and generate report (no submit)",
    }
)
def submit(
    ctx,
    omnibus_pipeline_dir,
    pattern="datadog-agent-7*.msi",
    output="vt-junit.xml",
    upload=True,
    sha256=None,
):
    """Scan an MSI with VirusTotal, build JUnit XML, and optionally upload to Datadog.

    JUnit format follows Datadog Test Visibility guidance. See:
    https://docs.datadoghq.com/tests/setup/junit_xml?tab=gitlab
    """
    import asyncio  # lazily import to avoid startup overhead for everyone

    async def _async_vt_scan():
        """Async wrapper for VirusTotal operations."""
        apikey = _ensure_vt_api_key(ctx)

        artifact = None
        if sha256:
            file_sha = sha256
        else:
            artifact = _find_artifact(omnibus_pipeline_dir, pattern)
            file_sha = _sha256_file(artifact)

        if sha256:
            print(f"Fetching VirusTotal report for SHA256: {file_sha}")
            file_report = await _fetch_file_report(apikey, file_sha)
        else:
            print(f"Scanning {artifact}")
            print(f"SHA256: {file_sha}")

            analysis_id = await _submit_scan(
                apikey,
                artifact,
            )
            print(f"Submitted file for analysis, analysis id: {analysis_id}")

            await _poll_analysis(apikey, analysis_id)
            print(f"Analysis completed, fetching file report for {file_sha}")

            file_report = await _fetch_file_report(apikey, file_sha)

        print(f"Goto https://www.virustotal.com/gui/file/{file_sha} for more details")

        return artifact, file_sha, file_report

    with gitlab_section("VirusTotal scan", collapsed=True):
        try:
            artifact, file_sha, file_report = asyncio.run(_async_vt_scan())
        except Exception as e:
            raise Exit(message=f"Error scanning VirusTotal: {e}") from e

        attributes = file_report.get("attributes", file_report)
        malicious, suspicious = _write_junit_xml(
            Path(output),
            (artifact.name if artifact else (attributes.get("meaningful_name") or file_sha)),
            file_sha,
            file_report,
        )
        print(f"JUnit XML written to {output}")

        if upload:
            # Upload directly through datadog-ci
            try:
                ddci = get_datadog_ci_command()
            except FileNotFoundError as e:
                raise Exit(message=str(e)) from None
            # Use a few helpful tags
            tags = [
                "--service",
                "datadog-agent",
                "--tags",
                "security:virustotal",
                "--tags",
                f"file.sha256:{file_sha}",
            ]
            # Passes --logs to make the VT file analysis output available
            cmd = [ddci, "junit", "upload", "--logs", *tags, output]
            env = os.environ.copy()
            # Isolate HOME to avoid CI gitconfig access issues
            with tempfile.TemporaryDirectory(prefix="junit-upload-") as iso:
                env2 = env | {"HOME": iso, "TEMP": iso, "TMP": iso, "TMPDIR": iso}
                cmd_str = " ".join(shlex.quote(p) for p in cmd)
                ctx.run(cmd_str, env=env2)
            print("Uploaded JUnit results to Datadog")

        if malicious > 0 or suspicious > 0:
            raise Exit(
                message=("Malicious or suspicious file detected! See JUnit for details"),
                code=1,
            )
