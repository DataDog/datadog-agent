"""Sequential, resumable Observer ablations through Agent CI API."""

import hashlib
import json
import math
import os
import random
import sqlite3
import tempfile
import time
import uuid
import zipfile
from copy import deepcopy
from pathlib import Path, PurePosixPath

import requests

from tasks.libs.anomalydetection.eval import (
    EXTRACTORS,
    _anchor_combos,
    _build_optuna_config,
    _full_stack_combo,
    random_component_combinations,
)

API_URL = "https://agent-ci-api.us1.ddbuild.io/internal/agent-ci-api/observer-ablation/eval"
ARTIFACT_BUCKET = "observer-log-ad-eval-artifacts-ddbuild"


def write_json(path: Path, value) -> None:
    """Replace a checkpoint atomically; readers never see a partial JSON document."""
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    with temporary.open("w", encoding="utf-8") as output:
        json.dump(value, output, indent=2, allow_nan=False)
        output.flush()
        os.fsync(output.fileno())
    temporary.replace(path)


def read_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def restore_checkpoint(job_id: str, output: Path) -> None:
    """Restore ablation checkpoints from an explicitly selected GitLab job's artifacts."""
    if not job_id.isdigit():
        raise ValueError("OBSERVER_ABLATION_RESUME_JOB_ID must be a numeric job ID")
    if output.exists():
        raise ValueError(f"Refusing to replace existing checkpoint directory: {output}")
    base = os.environ["CI_API_V4_URL"]
    project = os.environ["CI_PROJECT_ID"]
    headers = {"JOB-TOKEN": os.environ["CI_JOB_TOKEN"]}
    with requests.get(
        f"{base}/projects/{project}/jobs/{job_id}/artifacts", headers=headers, stream=True, timeout=60
    ) as response:
        response.raise_for_status()
        with tempfile.TemporaryFile() as download:
            for chunk in response.iter_content(1024 * 1024):
                download.write(chunk)
            download.seek(0)
            with zipfile.ZipFile(download) as archive:
                for item in archive.infolist():
                    path = PurePosixPath(item.filename)
                    if path.is_absolute() or ".." in path.parts or "\\" in item.filename:
                        raise ValueError("Invalid path in job artifacts")
                    if not path.parts or path.parts[0] != output.name or item.is_dir():
                        continue
                    # The JSON checkpoint is sufficient to reconstruct the optimizer and pending workflows.
                    # Do not restore the SQLite lock or unpickle any artifact.
                    if path.suffix not in {".json", ".md"}:
                        continue
                    destination = output.joinpath(*path.parts[1:])
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    destination.write_bytes(archive.read(item))
    if not (output / "study.json").is_file():
        raise ValueError(f"Job {job_id} has no ablation checkpoint")


def fingerprint(value) -> str:
    return hashlib.sha256(json.dumps(value, sort_keys=True, allow_nan=False).encode()).hexdigest()


def metric(metrics: dict, name: str) -> float:
    for key in (f"summary:mean_{name}", name):
        value = metrics.get(key)
        if isinstance(value, bool):
            continue
        try:
            number = float(value)
        except (TypeError, ValueError):
            continue
        if math.isfinite(number) and 0 <= number <= 1:
            return number
    raise ValueError(f"DDEval result has no finite {name} in [0, 1]")


class TransientError(RuntimeError):
    pass


class AgentCIClient:
    def __init__(self, token, *, url=API_URL):
        self.token = token
        self.url = url

    def post(self, attributes: dict, *, result=False, timeout=30):
        suffix = "/result" if result else ""
        kind = "result" if result else "workflow"
        try:
            response = requests.post(
                self.url + suffix,
                json={"data": {"type": f"observer_ablation_eval_{kind}_request", "attributes": attributes}},
                headers={"Authorization": self.token(), "X-DdOrigin": os.environ.get("CI_JOB_ID", "observer-ablation")},
                timeout=timeout,
            )
        except requests.RequestException as error:
            raise TransientError(f"Agent CI API connection failed: {type(error).__name__}") from error
        if response.status_code == 429 or response.status_code >= 500:
            raise TransientError(f"Agent CI API returned HTTP {response.status_code}: {response.text[:500]}")
        response.raise_for_status()
        data = response.json().get("data")
        if not isinstance(data, dict) or not isinstance(data.get("attributes"), dict):
            raise ValueError("Agent CI API returned an invalid JSON:API response")
        return {**data["attributes"], "id": data.get("id")}


class RemoteStudy:
    """Checkpoint every submission/result and keep a single experiment in flight."""

    def __init__(self, output: Path, spec: dict, client: AgentCIClient, *, resume=False, timeout=7200):
        self.output = output
        self.spec = spec
        self.client = client
        self.timeout = timeout
        self.output.mkdir(parents=True, exist_ok=True)
        self.lock = sqlite3.connect(str(output / ".lock.sqlite"), timeout=0)
        try:
            # OS-backed SQLite lock is released even when the driver is killed.
            self.lock.execute("BEGIN EXCLUSIVE")
            path = output / "study.json"
            if path.exists():
                if not resume:
                    raise ValueError(f"{output} already contains a study; use --resume or a new output directory")
                self.state = read_json(path)
                if self.state["spec"] != spec:
                    raise ValueError("Cannot resume: dataset, binary, search settings, or driver version changed")
            else:
                if resume:
                    raise ValueError(f"No study checkpoint in {output}")
                self.state = {
                    "id": str(uuid.uuid4()),
                    "spec": spec,
                    "dataset_version": spec["dataset_version"],
                    "trials": {},
                }
                self.save()
        except BaseException:
            self.lock.close()
            raise

    def __enter__(self):
        return self

    def __exit__(self, error_type, error, _traceback):
        try:
            if error_type is not None:
                write_json(
                    self.output / "report.json",
                    {
                        "status": "incomplete",
                        "error": str(error),
                        **self.state,
                    },
                )
                (self.output / "summary.md").write_text(
                    f"# Incomplete Observer ablation\n\n{error}\n\n"
                    "See study.json for completed results and pending workflow IDs.\n",
                    encoding="utf-8",
                )
        finally:
            self.lock.close()

    def save(self):
        write_json(self.output / "study.json", self.state)

    def evaluate(self, label: str, config: dict) -> float:
        trials = self.state["trials"]
        entry = trials.get(label)
        if entry is None:
            workflow_id = str(uuid.uuid5(uuid.UUID(self.state["id"]), label))
            experiment = deepcopy(self.spec["experiment_config"])
            inputs = experiment.setdefault("input_parameters", {})
            # Sampled component settings override the base settings, retaining unrelated options.
            base = deepcopy(inputs.get("testbench_config") or {})
            for key, value in config.items():
                if key == "components":
                    components = base.setdefault("components", {})
                    for name, settings in value.items():
                        components[name] = {**components.get(name, {}), **settings}
                else:
                    base[key] = value
            inputs["testbench_config"] = base
            inputs["trial_metadata"] = {
                **(inputs.get("trial_metadata") or {}),
                "study_id": self.state["id"],
                "trial": label,
                "seed": self.spec["seed"],
                "source_commit": self.spec["source_commit"],
            }
            request = {
                "workflow_id": workflow_id,
                "dataset_name": self.spec["dataset"],
                "dataset_version": self.state["dataset_version"],
                "experiment_config": json.dumps(experiment),
                "dataset_filter_json": json.dumps(self.spec["dataset_filter"]),
                "scenario_concurrency": self.spec["jobs"],
                "max_attempts": 1,
            }
            entry = {
                "config": config,
                "request": request,
                "workflow_id": workflow_id,
                "status": "prepared",
                "created_at": time.time(),
            }
            trials[label] = entry
            self.save()
            write_json(self.output / label / "config.json", base)
        elif entry["config"] != config:
            raise ValueError(f"Cannot resume {label}: optimizer produced a different config")

        if entry["status"] == "completed":
            return entry["score"]
        if entry["status"] == "failed":
            raise RuntimeError(f"{label} previously failed: {entry['error']}. Start a new study after resolving it.")

        workflow_id = entry["workflow_id"]
        print(f"{label}: workflow {workflow_id}", flush=True)
        deadline = time.monotonic() + self.timeout
        if entry["status"] == "prepared":
            # Save BEFORE sending: a lost response must never cause a second submission.
            entry["status"] = "submitted"
            self.save()
            try:
                started = self.client.post(entry["request"], timeout=min(120, self.timeout))
            except TransientError:
                print("Submission response unavailable; recovering by workflow ID through result polling.", flush=True)
            else:
                if started.get("id") != workflow_id:
                    raise RuntimeError("Agent CI API did not preserve the requested workflow ID")
                entry["run_id"] = started.get("run_id", "")
                self.save()

        retry_delay = 1
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError(
                    f"{label} did not complete within {self.timeout}s. Workflow {workflow_id} may still be running; "
                    "resume this study to continue polling. An uncertain submission is never automatically resubmitted."
                )
            try:
                result = self.client.post(
                    {
                        "workflow_id": workflow_id,
                        "run_id": entry.get("run_id", ""),
                        "wait_seconds": min(10, int(remaining)),
                    },
                    result=True,
                    timeout=min(15, remaining),
                )
            except TransientError as error:
                entry["last_poll_error"] = str(error)
                self.save()
                print(f"{label}: {error}; retrying result lookup", flush=True)
                retry_delay = min(30, retry_delay * 2)
            else:
                retry_delay = 1
                if result.get("completed"):
                    entry["result"] = result
                    entry["duration_s"] = time.time() - entry["created_at"]
                    try:
                        if result.get("status") != "completed":
                            raise ValueError(result.get("error") or f"workflow status: {result.get('status')}")
                        version = int(result.get("dataset_version") or 0)
                        if version <= 0:
                            raise ValueError(
                                "Result is missing its resolved dataset version; deploy the dataset API first"
                            )
                        if self.state["dataset_version"] not in (0, version):
                            raise ValueError("DDEval evaluated a different dataset version")
                        self.state["dataset_version"] = version
                        metrics = json.loads(result.get("metrics_json") or "{}")
                        score = metric(metrics, "f1")
                        if any(float(metrics.get(key) or 0) > 0 for key in ("timed_out", "summary:mean_timed_out")):
                            raise ValueError("Some scenarios timed out; refusing to optimize an incomplete evaluation")
                    except (ValueError, TypeError) as error:
                        entry.update(status="failed", error=str(error))
                        self.save()
                        raise RuntimeError(f"{label}: {error}") from error
                    entry.update(status="completed", score=score, metrics=metrics)
                    entry.pop("last_poll_error", None)
                    self.save()
                    write_json(self.output / label / "report.json", entry)
                    print(f"{label}: F1={score:.4f} {result.get('experiment_url', '')}", flush=True)
                    return score
            time.sleep(min(retry_delay, max(0, deadline - time.monotonic())))


def run_bayesian(remote: RemoteStudy, prefix: str, components: list[str], n_trials: int, seed: int, locked=()):
    import optuna

    optuna.logging.set_verbosity(optuna.logging.WARNING)
    study = optuna.create_study(direction="maximize", sampler=optuna.samplers.TPESampler(seed=seed))

    def objective(trial):
        config = _build_optuna_config(trial, components, set(locked))
        return remote.evaluate(f"{prefix}/trial_{trial.number:03d}", config)

    # Replaying completed objectives reconstructs TPE's RNG/history without loading a pickle.
    # Cached results return immediately; only the interrupted/new trial calls the API.
    study.optimize(objective, n_trials=n_trials)
    best_label = f"{prefix}/trial_{study.best_trial.number:03d}"
    return {"score": study.best_value, "trial": best_label, "components": components}


def run_ablation(output: Path, spec: dict, client: AgentCIClient, *, resume=False, timeout=7200):
    """Run combination search and tune its winner, never overlapping experiments."""
    with RemoteStudy(output, spec, client, resume=resume, timeout=timeout) as remote:
        rng = random.Random(spec["seed"])
        if spec.get("components"):
            winner = run_bayesian(
                remote, "tune", spec["components"], spec["n_trials_tune"], spec["seed"], spec.get("locked", [])
            )
            searches = []
        else:
            enable, disable = spec["force_enable"], spec["force_disable"]
            fixed = [_full_stack_combo(force_enable=enable, force_disable=disable)]
            fixed += _anchor_combos(force_enable=enable, force_disable=disable)
            combos = []
            seen = set()
            for combo in fixed:
                key = (tuple(combo["detectors"]), tuple(combo["correlators"]))
                if key not in seen:
                    seen.add(key)
                    combos.append(combo)
            combos = combos[: spec["n_combos"]]
            combos += random_component_combinations(
                max(0, spec["n_combos"] - len(combos)),
                seed=rng.randrange(2**32),
                force_enable=enable,
                force_disable=disable,
                exclude_combo_keys=seen,
            )
            searches = []
            for index, combo in enumerate(combos):
                components = sorted(
                    set(combo["detectors"] + combo["correlators"] + [e for e in EXTRACTORS if e not in disable])
                )
                for run in range(spec["m_runs"]):
                    searches.append(
                        run_bayesian(
                            remote,
                            f"search/combo_{index:03d}/run_{run:03d}",
                            components,
                            spec["n_trials_search"],
                            rng.randrange(2**32),
                        )
                    )
            best = max(searches, key=lambda item: item["score"])
            tuned = run_bayesian(remote, "tune", best["components"], spec["n_trials_tune"], rng.randrange(2**32))
            # Extra tuning may not improve on the search; never discard the best observed config.
            winner = max((best, tuned), key=lambda item: item["score"])
        best_entry = remote.state["trials"][winner["trial"]]
        report = {
            "status": "completed",
            "study_id": remote.state["id"],
            "spec": spec,
            "dataset_version": remote.state["dataset_version"],
            "best": winner,
            "searches": searches,
            "trials": remote.state["trials"],
        }
        write_json(output / "report.json", report)
        best_config = json.loads(best_entry["request"]["experiment_config"])["input_parameters"]["testbench_config"]
        write_json(output / "best_config.json", best_config)
        lines = [
            "# Observer ablation",
            "",
            f"Dataset: {spec['dataset']} (version {remote.state['dataset_version']})",
            f"Seed: {spec['seed']}",
            f"Best F1: {winner['score']:.4f}",
            "",
            "| Trial | F1 | Precision | Recall | Elapsed seconds | Experiment |",
            "| --- | ---: | ---: | ---: | ---: | --- |",
        ]
        for label, entry in remote.state["trials"].items():
            diagnostics = []
            for name in ("precision", "recall"):
                try:
                    diagnostics.append(f"{metric(entry['metrics'], name):.4f}")
                except ValueError:
                    diagnostics.append("—")
            url = entry["result"].get("experiment_url", "")
            lines.append(
                f"| {label} | {entry['score']:.4f} | {' | '.join(diagnostics)} | {entry['duration_s']:.1f} | [Open]({url}) |"
            )
        (output / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
        print(f"Best F1: {winner['score']:.4f}; config: {output / 'best_config.json'}", flush=True)
        return report
