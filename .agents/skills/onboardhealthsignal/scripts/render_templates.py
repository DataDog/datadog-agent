#!/usr/bin/env python3
"""Render the Go scaffold for a new Agent Health signal from a JSON spec.

Usage: python3 -I scripts/render_templates.py --spec spec.json --worktree <datadog-agent worktree>
           [--dry-run] [--only release-note,module]

Writes (Path A): issues/<pkg>/{module,issue,check,check_noop}.go, issue_test.go, check_test.go,
detect_test.go, module_test.go, comp/healthplatform/<pkg>_integration_test.go, and inserts the bundle.go
blank import; plus releasenotes/notes/<slug>.yaml ONLY when detection is implemented. Path B:
issues/<pkg>/<type>_issue.go + issue_test.go + integration test (no module.go, no bundle import).
BUILD.bazel is NEVER rendered: run gazelle.

Spec keys: pkg, pkg_dir (default pkg), path (A|B), year, issue_name, issue_id, category, location,
source, severity (LOW|MEDIUM|HIGH), check_kind (periodic|startup|none), interval_minutes, check_source,
build_tag, context_keys [snake_case...], title, title_args, description, description_args, summary,
steps [text with {key} tokens], tags, extra_static {k: v}, sample_context {k: v}, release_note_condition,
type_struct, module_struct (both default from issue_name, e.g. "Widget Probe Failed" -> WidgetProbeFailedIssue).
Options (see references/spec-fields.md): needs_config, id_scope (daemonset|host), multi_instance,
consecutive_failures (hysteresis, default 1; a found-but-unconfirmed instance returns an error so the scheduler
never resolves it), hold_minutes (keep reporting an instance that long after it was last seen, with an injectable
clock), recovery (automatic|restart|manual|conditional|held_window; held_window is the default with hold_minutes), platforms (["Linux", "macOS"]: scope sentence in the
release note), unix_socket_tests (true adds `&& linux` to detect tests), gate_config {k: v} and gate_env {k: v}
(gate the module test through config/env), unmet_requirements [..] (required for detection=partial), and
detection (todo|partial|implemented), co_owner (feature team handle for the printed CODEOWNERS recommendation),
step1_path_varies (step 1 may be a search instruction; see spec-fields.md).
The output lists codeowners_recommendation and pr_review_checklist: print them in the report and the PR body.
Path B owner adapter: owner_dir (repo relative) and owner_pkg write <owner_dir>/<issue_type>_health.go (a reporter
with an injectable store interface) and its tests.

detection=todo (default): the module registers NO built-in check (a registered check that cannot report only
burns a goroutine and ticker everywhere), NO release note is written (use the changelog/no-changelog label), and
the scaffold carries TODO(onboardhealthsignal) markers. detection=implemented registers the declared check, writes
the note, and leaves NO TODO markers and NO skipped scaffolds. detection=partial is implemented with a stated unmet
requirement (unmet_requirements): the limitation lands in the release note, in the issue description (never as a remediation step), and in
the printed PR snippets. Set detection after detectCondition works and re-render the wiring with
`--only module,release-note`.
Prints go_test_tags, integration_test_name and run_hint so the runner never guesses the test name.
"""
import argparse
import hashlib
import json
import re
import sys
from pathlib import Path

sys.dont_write_bytecode = True  # never write __pycache__ into the skill dir

SKILL = Path(__file__).resolve().parent.parent
TPL = SKILL / "references" / "templates"
IMPORT_BASE = "github.com/DataDog/datadog-agent"
PLACEHOLDER = re.compile(r"__([A-Z][A-Z0-9_]*?)__")
VERB = re.compile(r"%[#+\- 0]*\d*(?:\.\d+)?[a-zA-Z%]")
RECOVERY_TEXT = {
    "automatic": "The issue clears once the condition is no longer detected.",
    "restart": "The issue clears once the condition is no longer detected, but the Agent must be restarted to restore normal operation.",
    "manual": "The issue clears once the condition is no longer detected; a manual fix is required first.",
    "held_window": ("The issue stays active for the hold window after the condition was last seen, then clears; "
                    "restarting the Agent or fixing the cause does not clear it sooner."),
    "conditional": ("The issue clears once the condition is no longer detected. If the dependency was missing when the Agent "
                    "started, restart the Agent to restore normal operation; if it was lost later, the Agent reconnects on its own."),
}
GO_PREDECLARED = {"error", "string", "int", "bool", "len", "cap", "map", "type", "func", "range", "var", "const",
                  "new", "make", "copy", "append", "any", "nil", "true", "false", "context", "ctx", "err", "issue",
                  "extra", "steps", "t"}


def pascal(s):
    return "".join(p.capitalize() for p in s.split("_"))


def camel(s):
    p = pascal(s)
    return p[0].lower() + p[1:]


def var_name(key):
    name = camel(key)
    return name + "Value" if name in GO_PREDECLARED else name


def go_str(s):
    return json.dumps(s, ensure_ascii=False)


def sprintf_expr(fmt, args):
    """fmt.Sprintf, except a lone %s over one string argument becomes concatenation (perfsprint)."""
    if not args:
        return go_str(fmt)
    verbs = [m for m in VERB.findall(fmt) if m != "%%"]
    if len(verbs) == 1 and verbs[0] == "%s" and len(args) == 1 and "%%" not in fmt:
        pre, post = fmt.split("%s")
        parts = ([go_str(pre)] if pre else []) + [args[0]] + ([go_str(post)] if post else [])
        return " + ".join(parts)
    return f"fmt.Sprintf({go_str(fmt)}, {', '.join(args)})"


def step_text_expr(text):
    keys = re.findall(r"\{([a-z0-9_]+)\}", text)
    if not keys:
        return go_str(text)
    fmt = re.sub(r"\{[a-z0-9_]+\}", "%s", text)
    return sprintf_expr(fmt, [var_name(k) for k in keys])


def expand_format(fmt, arg_values):
    """Evaluate a Go format string over known string values (%s bare, %q quoted, %% literal)."""
    values = list(arg_values)
    out, i = [], 0
    for m in re.finditer(r"%[sq%]", fmt):
        out.append(fmt[i:m.start()])
        if m.group(0) == "%%":
            out.append("%")
        else:
            val = values.pop(0)
            out.append(json.dumps(val, ensure_ascii=False) if m.group(0) == "%q" else val)
        i = m.end()
    out.append(fmt[i:])
    return "".join(out)


def expand_step(text, ctx):
    return re.sub(r"\{([a-z0-9_]+)\}", lambda m: ctx[m.group(1)], text)


def words(name):
    return re.findall(r"[A-Za-z0-9]+", name)


def lower_camel(s):
    m = re.match(r"^([A-Z]+)(?=[A-Z][a-z])", s)
    if m:
        return m.group(1).lower() + s[m.end():]
    return s[0].lower() + s[1:]


def default_type_struct(issue_name):
    return "".join(w[0].upper() + w[1:] for w in words(issue_name)) + "Issue"


def noop_tag(expr):
    return f"!({expr})" if re.search(r"[\s&|]", expr) else f"!{expr}"


def derive_issue_type(name):
    return name.lower().replace(" ", "_")


def go_map_string(d):
    return "map[string]string{" + ", ".join(f"{go_str(k)}: {go_str(v)}" for k, v in d.items()) + "}"


def go_map_any(d):
    return "map[string]any{" + ", ".join(f"{go_str(k)}: {go_str(v)}" for k, v in d.items()) + "}"


def lower_first(s):
    return s[0].lower() + s[1:]


def check_parts(multi, hyst, hold_min, detection, needs_config, selfident, interval_const):
    """Go fragments for check.go and check_test.go; hysteresis, hold and keys are emitted only when used."""
    p = {}
    if multi:
        p["INSTANCE_BLOCK"] = (
            "// instance is one occurrence of the condition. Key tells several occurrences on the same host apart;\n"
            "// choose a bounded key, because the number of backend issues equals the number of distinct keys (one per\n"
            "// source, service, or entity; never one per unbounded value such as every file path under a wildcard).\n"
            "type instance struct {\n\tKey     string\n\tContext map[string]string\n}\n\n"
            "func (in instance) key() string { return in.Key }\n")
        p["ID_PARAM"], p["ID_ARG"] = "key string", "in.key()"
        p["ID_ARG_EMPTY"] = '""'
        p["ID_MULTI_ASSERTS"] = ('\tassert.NotEqual(t, a.instanceIssueID("one"), a.instanceIssueID("two"), "different instances must not share an ID")\n'
                                 '\tassert.NotEqual(t, a.instanceIssueID(""), a.instanceIssueID("one"))\n')
        p["ID_KEY_WRITE"] = '\tif key != "" {\n\t\t_, _ = h.Write([]byte{0})\n\t\t_, _ = h.Write([]byte(key))\n\t}\n'
    else:
        p["INSTANCE_BLOCK"] = (
            "// instance is one occurrence of the condition on this host (the signal exists at most once per host).\n"
            "type instance struct {\n\tContext map[string]string\n}\n\nfunc (instance) key() string { return \"\" }\n")
        p["ID_PARAM"], p["ID_ARG"], p["ID_KEY_WRITE"] = "", "", ""
        p["ID_ARG_EMPTY"], p["ID_MULTI_ASSERTS"] = "", ""
    std = ["\t\"context\"\n", "\t\"fmt\"\n", "\t\"hash/fnv\"\n"]
    if hyst:
        std.append("\t\"errors\"\n")
    if hyst or hold_min:
        std.append("\t\"sync\"\n")
    if hold_min:
        std.append("\t\"time\"\n")
    p["STD_IMPORTS"] = "".join(sorted(std))
    p["PENDING_ERR"] = ('// errPendingConfirmation tells the scheduler "could not decide yet": a non-nil error leaves every active\n'
                        '// issue untouched. The scheduler seeds persisted issue IDs at start and resolves any ID the first run does\n'
                        '// not return, so staying silent (nil, nil) while an instance is still unconfirmed would resolve it.\n'
                        'var errPendingConfirmation = errors.New("instance seen but not yet confirmed; leaving issue state unchanged")\n\n') if hyst else ""
    fields, init = [], []
    if hyst:
        fields.append("\t// threshold is how many consecutive runs must see an instance before it is reported.\n"
                      "\tthreshold int\n\tstreak    map[string]int\n")
        init.append(f"\tc.threshold = {hyst}\n\tc.streak = map[string]int{{}}\n")
    if hold_min:
        fields.append("\t// hold keeps reporting an instance for this long after it was last seen; now is injectable for tests.\n"
                      "\thold     time.Duration\n\tnow      func() time.Time\n\tlastSeen map[string]heldInstance\n")
        init.append("\tc.hold = holdWindow\n\tc.now = time.Now\n\tc.lastSeen = map[string]heldInstance{}\n")
    if hyst or hold_min:
        fields.append("\tmu sync.Mutex\n")
    p["STATE_FIELDS"] = "".join(fields)
    p["STATE_INIT"] = "".join(init)
    p["HOLD_CONST"] = (f"// holdWindow is how long an instance keeps being reported after it was last seen.\n"
                       f"const holdWindow = {hold_min} * time.Minute\n\n") if hold_min else ""
    if hyst and hold_min:
        p["RUN_FILTER"] = ("\tinstances := c.held(c.confirmed(found))\n"
                           "\tif len(found) > 0 && len(instances) == 0 {\n\t\treturn nil, errPendingConfirmation\n\t}\n")
    elif hyst:
        p["RUN_FILTER"] = ("\tinstances := c.confirmed(found)\n"
                           "\tif len(found) > 0 && len(instances) == 0 {\n\t\treturn nil, errPendingConfirmation\n\t}\n")
    elif hold_min:
        p["RUN_FILTER"] = "\tinstances := c.held(found)\n"
    else:
        p["RUN_FILTER"] = "\tinstances := found\n"
    methods = ""
    if hyst:
        methods += (
            "// confirmed applies the hysteresis: an instance is reported once it has been seen on threshold consecutive\n"
            "// runs, and its count resets when a run misses it.\n"
            "func (c *checker) confirmed(found []instance) []instance {\n\tc.mu.Lock()\n\tdefer c.mu.Unlock()\n"
            "\tseen := make(map[string]int, len(found))\n\tvar out []instance\n\tfor _, in := range found {\n"
            "\t\tn := c.streak[in.key()] + 1\n\t\tseen[in.key()] = n\n\t\tif n >= c.threshold {\n\t\t\tout = append(out, in)\n\t\t}\n\t}\n"
            "\tc.streak = seen\n\treturn out\n}\n\n")
    if hold_min:
        methods += (
            "type heldInstance struct {\n\tinstance instance\n\tseenAt   time.Time\n}\n\n"
            "// held keeps reporting an instance for hold after it was last seen, so a historical signal (a counter that\n"
            "// stopped moving, an event that already ended) stays visible for a while; the issue resolves when the hold expires.\n"
            "func (c *checker) held(current []instance) []instance {\n\tc.mu.Lock()\n\tdefer c.mu.Unlock()\n"
            "\tnow := c.now()\n\tseen := make(map[string]bool, len(current))\n\tout := make([]instance, 0, len(current))\n"
            "\tfor _, in := range current {\n\t\tc.lastSeen[in.key()] = heldInstance{instance: in, seenAt: now}\n\t\tseen[in.key()] = true\n\t\tout = append(out, in)\n\t}\n"
            "\tfor key, h := range c.lastSeen {\n\t\tif seen[key] {\n\t\t\tcontinue\n\t\t}\n"
            "\t\tif now.Sub(h.seenAt) < c.hold {\n\t\t\tout = append(out, h.instance)\n\t\t} else {\n\t\t\tdelete(c.lastSeen, key)\n\t\t}\n\t}\n"
            "\treturn out\n}\n\n")
    if hyst:
        methods += ("// resetStreak forgets every sighting: a run that could not decide (a detect error) must not count toward\n"
                    "// a later confirmation, so only consecutive conclusive runs confirm an instance.\n"
                    "func (c *checker) resetStreak() {\n\tc.mu.Lock()\n\tdefer c.mu.Unlock()\n\tc.streak = map[string]int{}\n}\n\n")
        p["DETECT_ERR_RESET"] = "\t\tc.resetStreak()\n"
    else:
        p["DETECT_ERR_RESET"] = ""
    p["STATE_METHODS"] = methods
    if detection == "todo":
        p["DETECT_COMMENT"] = (
            "// detectCondition is the detection seam; it is a method so it can read the checker's dependencies (config,\n"
            "// hostname). TODO(onboardhealthsignal): implement it from the root-cause input and delete this marker; keep\n"
            "// it free of side effects.\n//\n")
    else:
        p["DETECT_COMMENT"] = ("// detectCondition is the detection seam; it is a method so it can read the checker's dependencies (config,\n"
                               "// hostname). Keep it free of side effects.\n//\n")
    # check_test.go fragments
    key_arg = "instances[i].Key" if multi else ""
    p["TEST_ID_CALL"] = f"c.instanceIssueID({key_arg})"
    p["TEST_RANGE"] = "i, r" if multi else "_, r"
    p["TEST_NO_HOLD"] = "\t\tc.hold = 0 // the hold window has its own test below\n" if hold_min else ""
    state_init = ""
    if hyst:
        state_init += "threshold: 1, streak: map[string]int{}, "
    if hold_min:
        state_init += "hold: time.Hour, now: time.Now, lastSeen: map[string]heldInstance{}, "
    p["TEST_STATE_INIT"] = state_init
    p["TEST_STD_IMPORTS"] = ("\t\"errors\"\n\t\"strings\"\n\t\"testing\"\n" + ("\t\"time\"\n" if hold_min else ""))
    extra = ""
    if hyst:
        extra += (
            "\nfunc TestHysteresisReturnsAnErrorUntilConfirmed(t *testing.T) {\n\tpresent := true\n"
            "\tc := newTestChecker(t, \"node-a\", func() ([]instance, error) {\n\t\tif present {\n\t\t\treturn __SAMPLE_INSTANCES__, nil\n\t\t}\n\t\treturn nil, nil\n\t})\n"
            "\tc.threshold = 2\n" + ("\tc.hold = 0 // isolate the hysteresis from the hold window\n" if hold_min else "") + "\n"
            "\t// Unconfirmed: an error, not (nil, nil), so a persisted issue seeded by the scheduler is not resolved.\n"
            "\tfirst, err := c.Run()\n\trequire.ErrorIs(t, err, errPendingConfirmation)\n\tassert.Nil(t, first)\n\n"
            "\tsecond, err := c.Run()\n\trequire.NoError(t, err)\n\tassert.NotEmpty(t, second, \"the second consecutive sighting reports\")\n\n"
            "\t// Healthy: (nil, nil) so the scheduler resolves, and the count resets.\n"
            "\tpresent = false\n\tgone, err := c.Run()\n\trequire.NoError(t, err)\n\tassert.Nil(t, gone)\n"
            "\tpresent = true\n\tagain, err := c.Run()\n\trequire.ErrorIs(t, err, errPendingConfirmation)\n\tassert.Nil(t, again, \"a miss resets the count\")\n}\n"
            "\nfunc TestHysteresisStreakResetsOnAnInconclusiveRun(t *testing.T) {\n\tbroken := false\n"
            "\tc := newTestChecker(t, \"node-a\", func() ([]instance, error) {\n\t\tif broken {\n\t\t\treturn nil, errors.New(\"probe failed\")\n\t\t}\n\t\treturn __SAMPLE_INSTANCES__, nil\n\t})\n"
            "\tc.threshold = 2\n" + ("\tc.hold = 0\n" if hold_min else "") + "\n"
            "\t_, err := c.Run()\n\trequire.ErrorIs(t, err, errPendingConfirmation)\n"
            "\tbroken = true\n\t_, err = c.Run()\n\trequire.Error(t, err)\n\tbroken = false\n"
            "\t// The inconclusive run reset the streak, so one more sighting is still unconfirmed.\n"
            "\t_, err = c.Run()\n\trequire.ErrorIs(t, err, errPendingConfirmation, \"an inconclusive run must reset the streak\")\n}\n")
    if hold_min:
        extra += (
            "\nfunc TestHoldKeepsReportingAfterTheLastSighting(t *testing.T) {\n\tpresent := true\n\tclock := time.Unix(1_700_000_000, 0)\n"
            "\tc := newTestChecker(t, \"node-a\", func() ([]instance, error) {\n\t\tif present {\n\t\t\treturn __SAMPLE_INSTANCES__, nil\n\t\t}\n\t\treturn nil, nil\n\t})\n"
            "\tc.now = func() time.Time { return clock }\n\tc.hold = time.Hour\n\n"
            "\tseen, err := c.Run()\n\trequire.NoError(t, err)\n\trequire.NotEmpty(t, seen)\n\n"
            "\tpresent = false\n\tclock = clock.Add(30 * time.Minute)\n\tduring, err := c.Run()\n\trequire.NoError(t, err)\n\tassert.NotEmpty(t, during, \"still held\")\n\n"
            "\tclock = clock.Add(31 * time.Minute)\n\tafter, err := c.Run()\n\trequire.NoError(t, err)\n\tassert.Nil(t, after, \"hold expired, so the scheduler resolves the issue\")\n}\n")
    p["TEST_EXTRA"] = extra
    return p


SEARCH_WORDS = re.compile(r"(?i)\b(search|look for|look in|find the|locate|grep for)\b")


def step1_notes(spec):
    """Remediation step 1 must be a literal diagnostic command. A search instruction is allowed only when the file
    path varies by install, and then it names the exact string to search and the default paths per OS."""
    step1 = spec["steps"][0] if spec.get("steps") else ""
    cmd = re.search(r"`[^`]+`", step1)
    out = []
    if not cmd:
        out.append("step 1 has no literal command in backticks: write a runnable diagnostic command (for example "
                   "`datadog-agent status`). Use a search instruction only when the file path varies by install.")
    elif SEARCH_WORDS.search(step1) and not spec.get("step1_path_varies"):
        out.append("step 1 reads as a search instruction: use a literal command, or set step1_path_varies=true and name the "
                   "exact string to search and the default paths per OS.")
    if spec.get("step1_path_varies") and not (re.search(r"[\"'`][^\"'`]{3,}[\"'`]", step1) and
                                              len(re.findall(r"Linux|macOS|Windows|/var/log|/opt/|C:\\", step1)) >= 2):
        out.append("step1_path_varies is set: step 1 must name the exact string to search (quoted) and the common default "
                   "paths per OS (at least two).")
    if re.search(r"(?i)\blogs?\b", step1) and not re.search(r"kubectl logs|journalctl", step1):
        out.append("step 1 mentions logs: the Agent logs to stdout on Kubernetes, so give `kubectl logs <agent-pod>` for "
                   "Kubernetes plus `journalctl` or the default log path per OS for hosts.")
    return out


def recovery_sentence(recovery, hold_min):
    if recovery == "held_window":
        return (f"The issue stays active for {hold_min} minutes after the condition was last seen, then clears; "
                "restarting the Agent or fixing the cause does not clear it sooner.")
    return RECOVERY_TEXT[recovery]


def limitation_text(spec):
    return "Limitation: " + "; ".join(spec.get("unmet_requirements", [])) + "."


def build_values(spec):
    path = spec.get("path", "A")
    pkg = spec["pkg"]
    keys = spec.get("context_keys", [])
    name = spec["issue_name"]
    declared = spec.get("check_kind", "periodic" if path == "A" else "none") if path == "A" else "none"
    detection = spec.get("detection", "todo")
    if detection not in ("todo", "partial", "implemented"):
        raise SystemExit("detection must be 'todo', 'partial' or 'implemented'")
    unmet = list(spec.get("unmet_requirements", []))
    if detection == "partial" and not unmet:
        raise SystemExit("detection=partial needs unmet_requirements: the stated requirement the detection does not meet")
    impl = detection in ("implemented", "partial")
    # a check that cannot report must not be registered: only register once detection exists
    kind = declared if impl else "none"
    limitation = ("Limitation: " + "; ".join(unmet) + ".") if detection == "partial" else ""
    if limitation:
        # the limitation is part of the issue description (what the issue can and cannot say), never a remediation step:
        # every step must be an action the customer can take
        spec = dict(spec, description=spec["description"] + " " + (limitation.replace("%", "%%") if spec.get("description_args") else limitation))
    steps_all = list(spec["steps"])
    has_check = path == "A" and declared != "none"   # checker scaffold files exist
    tag = spec.get("build_tag")
    type_struct = spec.get("type_struct", default_type_struct(name))
    type_base = type_struct[:-len("Issue")] if type_struct.endswith("Issue") and len(type_struct) > 5 else type_struct
    hold_spec = int(spec.get("hold_minutes", 0))
    recovery = spec.get("recovery", "held_window" if hold_spec else "automatic")
    if recovery not in RECOVERY_TEXT:
        raise SystemExit("recovery must be one of " + ", ".join(RECOVERY_TEXT))
    if recovery == "held_window" and not hold_spec:
        raise SystemExit("recovery=held_window needs hold_minutes (the hold window that keeps the issue active)")
    if hold_spec and recovery in ("automatic", "conditional"):
        raise SystemExit("hold_minutes is set, so the issue stays active for the window after the last sighting: "
                         "use recovery=held_window (or restart/manual), never an auto-clear recovery")
    if recovery != "automatic":
        text = " ".join([spec["description"], *spec["steps"]]).lower()
        if re.search(r"(clears|resolves|recovers|resolved) (on its own|automatically|by itself)|auto-?resolve", text):
            raise SystemExit(f"recovery={recovery}: the issue text must not say the issue clears on its own")
    v = {
        "YEAR": str(spec.get("year", 2026)),
        "PKG": pkg,
        "PKG_DIR": spec.get("pkg_dir", pkg),
        "PKG_VAR": camel(pkg),
        "PKG_ALIAS": "" if spec.get("pkg_dir", pkg) == pkg else pkg + " ",
        "ISSUE_NAME": name,
        "ISSUE_TYPE": derive_issue_type(name),
        "ISSUE_ID": spec["issue_id"],
        "TYPE_STRUCT": type_struct,
        "TYPE_BASE": type_base,
        "MODULE_STRUCT": spec.get("module_struct", lower_camel(type_base) + "Module"),
        "PKG_PASCAL": pascal(pkg),
        "REPORTER": f"{type_base}HealthReporter",
        "STORE_IFACE": lower_first(type_base) + "HealthStore",
        "CATEGORY": spec["category"],
        "LOCATION": spec["location"],
        "SOURCE": spec["source"],
        "SEVERITY": spec["severity"],
        "SUMMARY": spec["summary"],
        "TAGS": ", ".join(go_str(t) for t in spec.get("tags", [])),
        "BUILD_TAG_LINE": f"//go:build {tag}\n\n" if tag and has_check else "",
        "NOOP_BUILD_TAG_LINE": f"//go:build {noop_tag(tag)}\n\n" if tag and has_check else "",
        "RELEASE_NOTE_CONDITION": spec.get("release_note_condition", "the condition is detected"),
        "CONSECUTIVE": str(int(spec.get("consecutive_failures", 1))),
    }
    v["CONTEXT_KEY_CONSTS"] = "\n".join(f'\t{"contextKey" + pascal(k)} = "{k}"' for k in keys)
    reads = []
    for k in keys:
        var = var_name(k)
        reads.append(f'\t{var} := context[contextKey{pascal(k)}]\n\tif {var} == "" {{\n\t\t{var} = unknownValue\n\t}}')
    v["CONTEXT_READS"] = "\n".join(reads) if reads else "\t_ = context"
    iv = spec.get("impact_variants")
    if iv:
        if iv["context_key"] not in keys:
            raise SystemExit("impact_variants.context_key must be one of context_keys")
        v["IMPACT_BLOCK"] = (
            f'\timpact := {go_str(iv["otherwise"])}\n'
            f'\tif n := context[contextKey{pascal(iv["context_key"])}]; n != "" && n != "0" && n != unknownValue {{\n'
            f'\t\timpact = {go_str(iv["when_nonzero"])}\n\t}}\n\n')
    else:
        v["IMPACT_BLOCK"] = ""
    extra = [f"\t\tcontextKey{pascal(k)}: {var_name(k)}," for k in keys]
    extra += [f"\t\t{go_str(k)}: {go_str(val)}," for k, val in spec.get("extra_static", {}).items()
              if not (iv and k == "impact")]
    if iv:
        extra.append('\t\t"impact": impact,')
    v["EXTRA_FIELDS"] = "\n".join(extra)
    t_args = [var_name(a) for a in spec.get("title_args", [])]
    d_args = [var_name(a) for a in spec.get("description_args", [])]
    v["TITLE_EXPR"] = sprintf_expr(spec["title"], t_args)
    v["DESC_EXPR"] = sprintf_expr(spec["description"], d_args)
    v["STEPS"] = "\n".join(f"\t\t\t\t{{Order: {i}, Text: {step_text_expr(t)}}},"
                           for i, t in enumerate(steps_all, 1))
    sample = spec.get("sample_context", {})
    v["SAMPLE_CONTEXT"] = go_map_string(sample)

    # expected values for issue_test.go, derived from the same spec (AGENTS.md mandatory assertions)
    def case(label, ctx):
        real = ctx or {}
        full = {k: real.get(k) or "unknown" for k in keys}
        if iv:
            n = real.get(iv["context_key"])
            full["impact"] = iv["when_nonzero"] if (n and n not in ("0", "unknown")) else iv["otherwise"]
        title = expand_format(spec["title"], [full[a] for a in spec.get("title_args", [])])
        desc = expand_format(spec["description"], [full[a] for a in spec.get("description_args", [])])
        step1 = expand_step(spec["steps"][0], full)
        extra_want = {**{k: val for k, val in full.items() if k in keys or k == "impact"},
                      **{k: val for k, val in spec.get("extra_static", {}).items() if not (iv and k == "impact")}}
        ctx_lit = "nil" if ctx is None else go_map_string(ctx)
        return (f"\t\t{{\n\t\t\tname:            {go_str(label)},\n\t\t\tcontext:         {ctx_lit},\n"
                f"\t\t\twantTitle:       {go_str(title)},\n\t\t\twantDescription: {go_str(desc)},\n"
                f"\t\t\twantFirstStep:   {go_str(step1)},\n\t\t\twantExtra:       {go_map_any(extra_want)},\n\t\t}},")
    cases = [case("populated context", sample), case("empty context applies defaults", {}),
             case("nil context applies defaults", None)]
    if iv:
        k = iv["context_key"]
        cases.append(case(f"impact is claimed only when {k} is non-zero", {**sample, k: "3"}))
        cases.append(case(f"no impact is claimed when {k} is zero", {**sample, k: "0"}))
    v["ISSUE_TEST_CASES"] = "\n".join(cases)
    v["STEP_COUNT"] = str(len(steps_all))
    asserts = [f"\tassert.Equal(t, {go_str(val)}, issue.Extra.AsMap()[{go_str(k)}])" for k, val in sample.items() if k in keys]
    v["CONTEXT_ASSERTIONS"] = "\n".join(asserts) or "\t_ = issue"

    multi = bool(spec.get("multi_instance", False))
    needs_config = bool(spec.get("needs_config", False))
    id_scope = spec.get("id_scope", "daemonset")
    if id_scope not in ("daemonset", "host"):
        raise SystemExit("id_scope must be 'daemonset' or 'host'")
    selfident = id_scope != "host"
    consecutive = int(spec.get("consecutive_failures", 1))
    hyst = consecutive if consecutive > 1 else 0
    hold_min = int(spec.get("hold_minutes", 0))
    v.update(check_parts(multi, hyst, hold_min, detection, needs_config, selfident, None))
    v["SAMPLE_INSTANCES"] = ("[]instance{{Key: \"first\", Context: " + v["SAMPLE_CONTEXT"] + "}, {Key: \"second\", Context: "
                             + v["SAMPLE_CONTEXT"] + "}}") if multi else "[]instance{{Context: " + v["SAMPLE_CONTEXT"] + "}}"
    v["TEST_EXTRA"] = v["TEST_EXTRA"].replace("__SAMPLE_INSTANCES__", v["SAMPLE_INSTANCES"])
    cfg_imp = f'\t"{IMPORT_BASE}/comp/core/config"\n' if needs_config else ""
    host_imp = f'\thostnameinterface "{IMPORT_BASE}/comp/core/hostname/hostnameinterface/def"\n'
    self_imp = f'\t"{IMPORT_BASE}/comp/healthplatform/issueregistry/utils/selfident"\n' if selfident else ""
    v["CHECK_IMPORTS"] = (cfg_imp + host_imp + self_imp + f'\t"{IMPORT_BASE}/comp/healthplatform/issues"\n'
                          f'\trunnerdef "{IMPORT_BASE}/comp/healthplatform/runner/def"').rstrip("\n")
    v["NOOP_IMPORTS"] = (cfg_imp + host_imp + self_imp + f'\trunnerdef "{IMPORT_BASE}/comp/healthplatform/runner/def"').rstrip("\n")
    v["CHECKER_FIELDS"] = (("\tcfg       config.Component\n" if needs_config else "") + "\thost      hostnameinterface.Component\n"
                           + ("\tselfIdent *selfident.SelfIdent\n" if selfident else ""))
    params = (["cfg config.Component"] if needs_config else []) + ["host hostnameinterface.Component"] + (
        ["selfIdent *selfident.SelfIdent"] if selfident else [])
    v["CTOR_PARAMS"] = ", ".join(params)
    v["NOOP_PARAMS"] = ", ".join((["config.Component"] if needs_config else []) + ["hostnameinterface.Component"] + (
        ["*selfident.SelfIdent"] if selfident else []))
    v["CTOR_INIT"] = (("cfg: cfg, " if needs_config else "") + "host: host, " + ("selfIdent: selfIdent, " if selfident else "")).rstrip(", ")
    v["CTOR_ARGS"] = ", ".join((["deps.Config"] if needs_config else []) + ["deps.Hostname"] + (["deps.SelfIdent"] if selfident else []))
    disc_ident = "c.selfIdent" if selfident else "nil"
    v["DISCRIMINATOR_EXPR"] = f"issues.IssueDiscriminator({disc_ident}, c.host.GetSafe(context.Background()))"
    v["TEST_IMPORTS"] = f'\t"{IMPORT_BASE}/comp/core/config"\n' if needs_config else ""
    v["TEST_CHECKER_FIELDS"] = ("cfg: config.NewMock(t), " if needs_config else "") + "host: h, "
    v["DETECT_TEST_IMPORTS"] = v["TEST_IMPORTS"]
    v["DETECT_TEST_CHECKER_FIELDS"] = ("cfg: config.NewMock(t), " if needs_config else "") + "host: h"
    if spec.get("unix_socket_tests"):
        v["DETECT_BUILD_LINE"] = f"//go:build {tag} && linux\n\n" if tag else "//go:build linux\n\n"
    else:
        v["DETECT_BUILD_LINE"] = v["BUILD_TAG_LINE"]
    if detection == "todo":
        v["DETECT_TEST_BODY"] = (
            '\tt.Skip("TODO(onboardhealthsignal): exercise detectCondition against real objects, then delete this skip")\n\n'
            '\th, _ := hostnamemock.NewMock("node-a")\n'
            f'\tc := &checker{{{v["DETECT_TEST_CHECKER_FIELDS"]}}}\n\n'
            '\tinstances, err := c.detectCondition()\n\n\trequire.NoError(t, err)\n'
            '\tassert.Empty(t, instances, "set up the failing condition first, then assert the instances it yields")')
    else:
        v["DETECT_TEST_BODY"] = (
            '\th, _ := hostnamemock.NewMock("node-a")\n'
            f'\tc := &checker{{{v["DETECT_TEST_CHECKER_FIELDS"]}}}\n\n'
            '\t// Contract that holds for any implementation: no panic, and every instance carries a non-nil context.\n'
            '\t// Add cases for the real condition below (healthy, present, and each error class).\n'
            '\tinstances, err := c.detectCondition()\n\n'
            '\tif err == nil {\n\t\tfor _, in := range instances {\n\t\t\tassert.NotNil(t, in.Context)\n\t\t}\n\t}\n'
            '\t_ = require.New(t)')

    # release note: only when detection exists, and never promise recovery the consumer cannot deliver
    platforms = list(spec.get("platforms", []))
    platform_sentence = (f" Supported on {' and '.join(platforms)} only." if platforms else "")
    if impl:
        v["RELEASE_NOTE_TEXT"] = (f'Agent Health: the Agent now reports "{name}" when {v["RELEASE_NOTE_CONDITION"]}.{platform_sentence}\n'
                                  f"    The issue includes remediation steps. {recovery_sentence(recovery, hold_spec)}"
                                  + (f"\n    {limitation}" if limitation else ""))
    else:
        v["RELEASE_NOTE_TEXT"] = ""
    v["RELEASE_NOTE_HEADER"] = ""
    v["TEST_NAME"] = f"Test{type_base}SurvivesFullPipeline"
    v["FIND_FUNC"] = f"find{type_base}Issue"

    # issue.go imports and path-b consts
    v["ISSUE_IMPORTS"] = ('\t"fmt"\n\n\t"github.com/DataDog/agent-payload/v5/healthplatform"\n'
                          '\t"google.golang.org/protobuf/types/known/structpb"')
    v["PACKAGE_DOC"] = (f'// Package {pkg} builds the "{name}" Agent Health issue for components that report it directly.\n'
                        if path == "B" else "")  # Path A: module.go carries the package comment (revive package-comments)
    if path == "B":
        v["PATH_B_CONSTS"] = (
            "\nconst (\n\t// IssueName is the human-readable issue name and the template registry key.\n"
            f'\tIssueName = "{name}"\n\t// IssueType is IssueName lowercased with spaces replaced by underscores.\n'
            f'\tIssueType = "{derive_issue_type(name)}"\n\t// IssueID is the instance ID prefix; the reporting component appends a suffix.\n'
            f'\tIssueID = "{spec["issue_id"]}"\n)\n')
    else:
        v["PATH_B_CONSTS"] = ""

    # module.go (Path A)
    interval = int(spec.get("interval_minutes", 15))
    uses_time = kind == "periodic" and interval > 0
    v["MODULE_IMPORTS"] = (('\t"time"\n\n' if uses_time else "") +
                           '\t"github.com/DataDog/agent-payload/v5/healthplatform"\n\n'
                           f'\t"{IMPORT_BASE}/comp/healthplatform/issues"\n'
                           f'\trunnerdef "{IMPORT_BASE}/comp/healthplatform/runner/def"')
    # the scheduler key is only used by a registered check; declaring it earlier trips the `unused` linter
    ms = v["MODULE_STRUCT"]
    v["CHECKSOURCE_CONST"] = (f'\nconst checkSource = "{spec.get("check_source", pkg)}"\n' if kind != "none" else "")
    if kind == "periodic" and uses_time:
        v["CHECKSOURCE_CONST"] += f"\n// defaultInterval is how often the periodic check runs; tests inject another through newModule.\nconst defaultInterval = {interval} * time.Minute\n"
    v["MODULE_FIELDS"] = ("\tchecker *checker\n" if has_check else "") + ("\tinterval time.Duration\n" if (kind == "periodic" and uses_time) else "")
    if kind == "periodic" and uses_time:
        v["MODULE_CTOR"] = (f"\treturn newModule(deps, defaultInterval)\n}}\n\n"
                            f"func newModule(deps issues.ModuleDeps, interval time.Duration) issues.Module {{\n"
                            f"\treturn &{ms}{{\n\t\tchecker:  newChecker({v['CTOR_ARGS']}),\n\t\tinterval: interval,\n\t}}")
    else:
        v["MODULE_CTOR"] = (f"\treturn &{ms}{{\n\t\tchecker: newChecker({v['CTOR_ARGS']}),\n\t}}"
                            if has_check else f"\t_ = deps\n\treturn &{ms}{{}}")
    periodic = (f"// BuiltInPeriodicHealthCheck runs the detection on a schedule; Fn must return (nil, nil) when healthy.\n"
                f"func (m *{ms}) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {{\n"
                f"\treturn &runnerdef.BuiltInPeriodicHealthCheck{{\n\t\tBuiltInHealthCheck: runnerdef.BuiltInHealthCheck{{\n"
                f"\t\t\tSource: checkSource,\n\t\t\tFn:     m.checker.Run,\n\t\t}},\n"
                f"\t\tInterval: {'m.interval' if uses_time else '0'},\n\t}}\n}}\n")
    startup = (f"// BuiltInStartupHealthCheck runs the detection once at startup (condition changes only on restart).\n"
               f"func (m *{ms}) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {{\n"
               f"\treturn &runnerdef.BuiltInHealthCheck{{\n\t\tSource: checkSource,\n\t\tFn:     m.checker.Run,\n\t}}\n}}\n")
    todo_note = ("// TODO(onboardhealthsignal): detection is not implemented, so no built-in check is registered (a\n"
                 "// registered check that cannot report only costs a goroutine and a ticker in every agent). Once\n"
                 "// detectCondition works, set detection to implemented and re-render this wiring.\n") if (
        has_check and detection == "todo") else ""
    nil_periodic = (f"{todo_note}func (m *{ms}) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {{\n\treturn nil\n}}\n")
    nil_startup = (f"func (m *{ms}) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {{\n\treturn nil\n}}\n")
    if kind == "periodic":
        v["CHECK_METHODS"] = periodic + "\n" + nil_startup
    elif kind == "startup":
        v["CHECK_METHODS"] = nil_periodic + "\n" + startup
    else:
        v["CHECK_METHODS"] = nil_periodic + "\n" + nil_startup

    # module_test.go
    imports = ['\t"testing"\n'] + (['\t"time"\n'] if uses_time else [])
    v["MODULE_TEST_STD_IMPORTS"] = "".join(imports)
    v["MODULE_TEST_CFG_IMPORT"] = f'\t"{IMPORT_BASE}/comp/core/config"\n' if needs_config else ""
    v["MODULE_TEST_DEPS"] = "issues.ModuleDeps{Hostname: h" + (", Config: config.NewMock(t)" if needs_config else "") + "}"
    if kind == "periodic":
        v["MODULE_TEST_CHECKS"] = ("\tperiodic := m.BuiltInPeriodicHealthCheck()\n\trequire.NotNil(t, periodic)\n"
                                   "\tassert.Equal(t, checkSource, periodic.Source)\n"
                                   f"\tassert.Equal(t, {interval} * time.Minute, periodic.Interval)\n"
                                   "\tassert.Equal(t, defaultInterval, periodic.Interval)\n"
                                   "\tassert.NotNil(t, periodic.Fn)\n\tassert.Nil(t, m.BuiltInStartupHealthCheck())\n\n"
                                   "\t// the interval is injectable so a scheduler-level test can use a short one\n"
                                   "\tfast := newModule(deps, time.Second).BuiltInPeriodicHealthCheck()\n"
                                   "\trequire.NotNil(t, fast)\n\tassert.Equal(t, time.Second, fast.Interval)")
    elif kind == "startup":
        v["MODULE_TEST_CHECKS"] = ("\tstartup := m.BuiltInStartupHealthCheck()\n\trequire.NotNil(t, startup)\n"
                                   "\tassert.Equal(t, checkSource, startup.Source)\n\tassert.NotNil(t, startup.Fn)\n"
                                   "\tassert.Nil(t, m.BuiltInPeriodicHealthCheck())")
    else:
        v["MODULE_TEST_CHECKS"] = ("\t// No built-in check is registered (externally reported, or detection is still a TODO).\n"
                                   "\tassert.Nil(t, m.BuiltInPeriodicHealthCheck())\n\tassert.Nil(t, m.BuiltInStartupHealthCheck())")

    # integration test
    v["REGISTRATION_NOTE"] = (
        "// store, the forwarder, and the payload as the intake receives it. Title and\n"
        "// remediation are populated only when the registry resolved the template; a\n"
        "// missing bundle.go import would ship the minimal fallback proto and fail here."
        if path == "A" else
        "// store, the forwarder, and the payload as the intake receives it. Path B calls\n"
        "// BuildIssue directly (there is no registry lookup), so this proves forwarding and\n"
        "// payload shape, not registration.")
    imp = f'\t{v["PKG_ALIAS"]}"{IMPORT_BASE}/comp/healthplatform/issues/{v["PKG_DIR"]}"\n'
    owner_driven = path == "B" and bool(spec.get("owner_dir") and spec.get("owner_pkg"))
    owner_imp = f'\towner "{IMPORT_BASE}/{spec.get("owner_dir", "")}"\n'
    v["PKG_IMPORT_LINE"] = "" if path == "A" else (owner_imp if owner_driven else imp)
    v["RESOLVE_BLOCK"], v["RESOLVED_HELPER"] = "", ""
    idc = f"{v['PKG_VAR']}IssueID"
    if path == "A":
        v["RUNNER_IMPORT"] = f'\trunnerdef "{IMPORT_BASE}/comp/healthplatform/runner/def"\n'
        v["RUNNER_DEP"] = "\t\tRunner runnerdef.Component\n"
        v["REPORT_BLOCK"] = (
            "\t// Drive the real runner with a synthetic detection so the report travels\n"
            "\t// runner -> registry.BuildIssue -> store -> egress -> fakeintake.\n"
            f"\t_, err := deps.Runner.Run({go_str(pkg + '-test')}, func() ([]runnerdef.IssueReport, error) {{\n"
            "\t\treturn []runnerdef.IssueReport{{\n"
            f"\t\t\tIssueID:   {idc} + \":integration-test\",\n"
            f"\t\t\tIssueName: {v['PKG_VAR']}IssueName,\n"
            f"\t\t\tContext:   {v['SAMPLE_CONTEXT']},\n"
            "\t\t}}, nil\n\t})\n\trequire.NoError(t, err)")
    elif owner_driven:
        v["RUNNER_IMPORT"] = ""
        v["RUNNER_DEP"] = ""
        v["REPORT_BLOCK"] = (
            "\t// Drive the owner's reporter against the real store so the whole owner-to-intake path is covered.\n"
            f"\treporter := owner.New{v['REPORTER']}(deps.Store, \"node-a\", nil)\n"
            f"\treporter.Observe(true, {v['SAMPLE_CONTEXT']})")
        v["RESOLVE_BLOCK"] = (
            "\n\t// Clearing the condition must reach the intake as RESOLVED.\n"
            "\treporter.Observe(false, nil)\n"
            f"\tawait{type_base}Resolved(t, fiClient)\n")
        v["RESOLVED_HELPER"] = (
            f"func await{type_base}Resolved(t *testing.T, fiClient *fakeintakeclient.Client) {{\n\tt.Helper()\n"
            "\trequire.Eventually(t, func() bool {\n\t\tpayloads, err := fiClient.GetAgentHealth()\n\t\tif err != nil {\n\t\t\treturn false\n\t\t}\n"
            "\t\tfor _, p := range payloads {\n"
            f"\t\t\tif iss := {v['FIND_FUNC']}(p.Issues); iss != nil && iss.GetPersistedIssue().GetState() == healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED {{\n"
            "\t\t\t\treturn true\n\t\t\t}\n\t\t}\n\t\treturn false\n"
            f"\t}}, 5*time.Second, 50*time.Millisecond, \"{v['ISSUE_TYPE']} issue never reached the RESOLVED state on the wire\")\n}}\n\n")
    else:
        v["RUNNER_IMPORT"] = ""
        v["RUNNER_DEP"] = ""
        v["REPORT_BLOCK"] = (
            f"\tissue, err := {pkg}.{v['TYPE_STRUCT']}{{}}.BuildIssue({v['SAMPLE_CONTEXT']})\n"
            "\trequire.NoError(t, err)\n"
            f"\tissue.Id = {idc} + \":integration-test\"\n"
            "\trequire.NoError(t, deps.Store.ReportIssue(issue))")
    sev = f"healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_{spec['severity']}"
    v["WIRE_SEVERITY"] = sev
    gate_cfg = spec.get("gate_config", {})
    gate_env = spec.get("gate_env", {})
    first_tick = int(spec.get("consecutive_failures", 1)) <= 1
    gate_notes = []
    can_gate = path == "A" and kind != "none" and (gate_cfg or gate_env) and first_tick and not tag
    if can_gate:
        env_lines = "".join(f"\tt.Setenv({go_str(k)}, {go_str(val)})\n" for k, val in gate_env.items())
        cfg_lines = "".join(f"\t\tcfg.SetInTest({go_str(k)}, {json.dumps(val)})\n" for k, val in gate_cfg.items())
        v["MODULE_PIPELINE_TEST"] = (
            f"// {v['TEST_NAME']}ThroughTheModule enables the gate through config and environment only\n"
            "// (without importing the issue package) and expects the issue on the first scheduler tick:\n"
            "// the scheduler runs each check once immediately at start.\n"
            f"func {v['TEST_NAME']}ThroughTheModule(t *testing.T) {{\n{env_lines}"
            f"\t_, fiClient := start{type_base}Pipeline(t, func(cfg config.Component) {{\n{cfg_lines}\t}})\n\n"
            f"\treceived := await{type_base}Issue(t, fiClient)\n"
            f"\tassert{type_base}Wire(t, received)\n}}\n")
    elif detection == "todo":
        v["MODULE_PIPELINE_TEST"] = (
            f"// {v['TEST_NAME']}ThroughTheModule is a scaffold. TODO(onboardhealthsignal): set the gate through\n"
            "// config and environment only (never import the issue package), start the bundle with the real\n"
            "// condition present, expect the issue on the first scheduler tick, and assert the wire fields.\n"
            f"func {v['TEST_NAME']}ThroughTheModule(t *testing.T) {{\n"
            "\tt.Skip(\"TODO(onboardhealthsignal): drive the real check through the gate, then delete this skip\")\n}\n")
    else:
        v["MODULE_PIPELINE_TEST"] = ""   # implemented or partial: never leave a skipped scaffold in repo tests
        if path == "A" and kind != "none":
            why = ("a build_tag is set (the root Bazel target may not carry it)" if tag else
                   "consecutive_failures > 1 (the issue cannot appear on the first tick)" if not first_tick else
                   "the gate is not a config key or environment variable")
            gate_notes.append(f"no gate-through-config test was rendered because {why}; write one by hand against the real gate "
                              "(expvar, file, socket), without importing the issue package, and expect the issue on the first tick.")
    v["_gate_notes"] = gate_notes
    v["REGISTRATION_COMMENT"] = (
        f"// {v['TEST_NAME']} exercises the whole pipeline: module registration through bundle.go, the registry\n"
        "// template lookup, the store, the forwarder, and the payload as the intake receives it. Title and\n"
        "// remediation are populated only when the registry resolved the template; a missing bundle.go import\n"
        "// would ship the minimal fallback proto and fail here."
        if path == "A" else
        f"// {v['TEST_NAME']} exercises the owner-to-intake path: the store, the forwarder,\n"
        "// and the payload as the intake receives it. The owner reports through BuildIssue directly (there is no\n"
        "// registry lookup, so this does not prove registration); the owner wiring is tested in its own package.")
    v["CONSTS_COMMENT"] = (
        "// The identity constants are literals on purpose: importing the issue package would run its init() and\n"
        "// register the module even without the blank import in bundle.go. They carry the package prefix because the\n"
        "// tests of every signal share this package."
        if path == "A" else
        "// The identity constants carry the package prefix because the tests of every signal share this package.")
    v["ISSUE_IMPORT_PATH"] = f"{IMPORT_BASE}/comp/healthplatform/issues/{v['PKG_DIR']}"
    v["OWNER_PKG"] = spec.get("owner_pkg", "")
    v["PR_TITLE_SUFFIX"] = (f" (partial: {'; '.join(unmet)})" if detection == "partial" else "")
    return v, path, kind, has_check, detection


def render(text, values, where=""):
    def sub(m):
        k = m.group(1)
        if k not in values:
            raise KeyError(f"unknown placeholder __{k}__ in {where}")
        return values[k]
    return PLACEHOLDER.sub(sub, text)


def insert_bundle_import(text, pkg_dir):
    line = f'\t_ "{IMPORT_BASE}/comp/healthplatform/issues/{pkg_dir}" // registers templates via init()'
    if f"/issues/{pkg_dir}\"" in text:
        return text
    rx = re.compile(r'(?m)^\t_ "' + re.escape(IMPORT_BASE) + r'/comp/healthplatform/issues/([^"]+)".*$')
    matches = list(rx.finditer(text))
    if not matches:
        raise SystemExit("bundle.go: no existing issue blank imports found; insert manually")
    for m in matches:
        if m.group(1) > pkg_dir:
            return text[:m.start()] + line + "\n" + text[m.start():]
    last = matches[-1]
    return text[:last.end()] + "\n" + line + text[last.end():]


def plan_files(spec, values, path, kind, has_check, detection):
    out = {}
    pkgdir = f"comp/healthplatform/issues/{values['PKG_DIR']}"
    read = lambda rel: (TPL / rel).read_text()
    if path == "A":
        out[f"{pkgdir}/module.go"] = render(read("path_a/module.go.tmpl"), values, "module.go")
        out[f"{pkgdir}/module_test.go"] = render(read("path_a/module_test.go.tmpl"), values, "module_test.go")
        out[f"{pkgdir}/issue.go"] = render(read("common/issue.go.tmpl"), values, "issue.go")
        if has_check:
            out[f"{pkgdir}/check.go"] = render(read("path_a/check.go.tmpl"), values, "check.go")
            out[f"{pkgdir}/check_test.go"] = render(read("path_a/check_test.go.tmpl"), values, "check_test.go")
            out[f"{pkgdir}/detect_test.go"] = render(read("path_a/detect_test.go.tmpl"), values, "detect_test.go")
            if spec.get("build_tag"):
                out[f"{pkgdir}/check_noop.go"] = render(read("path_a/check_noop.go.tmpl"), values, "check_noop.go")
    else:
        fname = values["ISSUE_TYPE"].replace("-", "_") + "_issue.go"
        out[f"{pkgdir}/{fname}"] = render(read("common/issue.go.tmpl"), values, "issue.go")
    out[f"{pkgdir}/issue_test.go"] = render(read("common/issue_test.go.tmpl"), values, "issue_test.go")
    if path == "B" and spec.get("owner_dir") and spec.get("owner_pkg"):
        stem = values["ISSUE_TYPE"].replace("-", "_")
        out[f"{spec['owner_dir']}/{stem}_health.go"] = render(read("path_b/reporter.go.tmpl"), values, "reporter.go")
        out[f"{spec['owner_dir']}/{stem}_health_test.go"] = render(read("path_b/reporter_test.go.tmpl"), values, "reporter_test.go")
    out[f"comp/healthplatform/{values['PKG']}_integration_test.go"] = render(
        read("common/integration_test.go.tmpl"), values, "integration_test.go")
    if detection in ("implemented", "partial"):
        slug = hashlib.sha1(values["ISSUE_TYPE"].encode()).hexdigest()[:16]
        out[f"releasenotes/notes/{values['PKG']}-health-issue-{slug}.yaml"] = render(
            read("common/release_note.yaml.tmpl"), values, "release_note")
    return out


def run_gofmt(wt, rels):
    """gofmt -w the rendered files when gofmt is on PATH (alignment/import order); report otherwise."""
    import shutil
    import subprocess
    exe = shutil.which("gofmt") or "/opt/homebrew/opt/go@1.25/bin/gofmt"
    if not Path(exe).exists():
        return "gofmt not found; run `gofmt -w` on the new files"
    r = subprocess.run([exe, "-w", *rels], cwd=wt, capture_output=True, text=True)
    return "ok" if r.returncode == 0 else f"gofmt error: {r.stderr.strip()[:300]}"


def only_filter(files, only):
    wanted = {o.strip() for o in only.split(",") if o.strip()}
    bad = wanted - {"release-note", "module"}
    if bad:
        raise SystemExit(f"--only accepts release-note,module (got {sorted(bad)})")
    def keep(rel):
        return (("release-note" in wanted and rel.startswith("releasenotes/")) or
                ("module" in wanted and (rel.endswith("/module.go") or rel.endswith("/module_test.go"))))
    return {k: v for k, v in files.items() if keep(k)}


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--spec", required=True)
    ap.add_argument("--worktree", required=True)
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--only", help="comma list of release-note,module: rewrite only these (after detection lands)")
    args = ap.parse_args(argv)
    spec = json.loads(Path(args.spec).read_text())
    values, path, kind, has_check, detection = build_values(spec)
    wt = Path(args.worktree)
    files = plan_files(spec, values, path, kind, has_check, detection)
    if args.only:
        files = only_filter(files, args.only)
    written = []
    for rel, content in files.items():
        dest = wt / rel
        if not args.dry_run:
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text(content)
        written.append(rel)
    if path == "A" and not args.only:
        bundle = wt / "comp/healthplatform/bundle.go"
        if bundle.exists():
            new = insert_bundle_import(bundle.read_text(), values["PKG_DIR"])
            if not args.dry_run:
                bundle.write_text(new)
            written.append("comp/healthplatform/bundle.go (import inserted)")
    gofmt = "skipped"
    if not args.dry_run:
        go_files = [w.split(" ")[0] for w in written if w.endswith(".go") or ".go " in w]
        gofmt = run_gofmt(wt, go_files) if go_files else "skipped (no Go files written)"
    tag = spec.get("build_tag")
    tags = f"{tag} test" if tag else "test"
    name = values["TEST_NAME"]
    notes = list(values["_gate_notes"])
    if "%q" in spec["title"] + spec["description"]:
        notes.append("title/description use %q: a catalog template cannot reproduce Go quoting for values that contain a quote or "
                     "backslash, so the catalog fixture only matches ordinary values; quote YAML front matter safely (single quotes, "
                     "doubled) and note the limit.")
    notes.extend(step1_notes(spec))
    if detection in ("partial", "implemented") and not spec.get("platforms"):
        notes.append("no platforms in the spec: the release note states no platform scope. Set platforms (for example "
                     "[\"Linux\"]) to the OSes where detection runs, or confirm it runs everywhere.")
    if detection == "partial":
        notes.append("detection=partial: put the limitation in the PR title suffix and the first paragraph of 'What does this PR do' "
                     "(see pr_title_suffix and pr_first_paragraph), not only in Additional Notes.")
    if path == "B" and spec.get("owner_dir"):
        notes.append(f"pass --owner-dir {spec['owner_dir']} to gazelle, tests and the linter, --owner-file to check_signal, and wire the "
                     "reporter with an optional store dependency (see the generated file header). bundle.go does not apply to Path B.")
    if detection == "todo":
        notes.append("detection=todo: no built-in check is registered and no release note is written; use the "
                     "changelog/no-changelog label. After implementing detectCondition set detection=implemented "
                     "and run again with --only module,release-note.")
    pkg_dir_name = values["PKG_DIR"]
    co_owner = spec.get("co_owner") or "@DataDog/<feature-team>"
    codeowners_line = f"/comp/healthplatform/issues/{pkg_dir_name}/ {co_owner} @DataDog/fleet-remediation"
    checklist = [f"CODEOWNERS (recommended, not applied): add `{codeowners_line}` outside the generated block, or drop it "
                 "if no feature team co-owns the code",
                 f"README: add a row for `{pkg_dir_name}` to the module table in comp/healthplatform/README.md "
                 "(the renderer does not write it)",
                 "Platform scope: the release note states where detection runs" if detection != "todo" else
                 "Release note: none while detection is todo; use the changelog/no-changelog label"]
    notes.append("print codeowners_recommendation in the report and list pr_review_checklist under a 'Review checklist' "
                 "heading in the PR body; never add the CODEOWNERS line silently.")
    print(json.dumps({"path": path, "check_kind": kind, "codeowners_recommendation": codeowners_line,
                      "pr_review_checklist": checklist, "declared_check_kind": spec.get("check_kind"),
                      "detection": detection, "files": written, "dry_run": args.dry_run, "gofmt": gofmt,
                      "go_test_tags": tags, "integration_test_name": name, "notes": notes,
                      "pr_title_suffix": values["PR_TITLE_SUFFIX"],
                      "pr_first_paragraph": (f'Reports "{spec["issue_name"]}" when {values["RELEASE_NOTE_CONDITION"]}. ' +
                                              (limitation_text(spec) if detection == "partial" else "")).strip(),
                      "run_hint": (f"dda inv test --targets=./comp/healthplatform --test-args='-test.run=^{name}$ -test.v' "
                                   f"--bazel-args='--test_output=all' (run gazelle first, or the file is not in the Bazel "
                                   f"target and nothing runs; confirm `--- PASS: {name}` in the output). Older SHAs lack "
                                   f"--test-args: probe `dda inv test --help` and use --test-run-name='^{name}$' -v there")}, indent=2))
    print("NEXT: gofmt -w the new files, run gazelle for BUILD.bazel, then check_signal.py", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
