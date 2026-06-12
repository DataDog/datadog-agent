export const meta = {
  name: 'deb-parity-loop',
  description: 'Adaptive plan/implement/evaluate loop driving the Bazel deb toward parity with reference-omnibus.deb',
  whenToUse: 'Closing structural gaps between the Bazel-built datadog-agent deb and the reference omnibus deb',
  phases: [
    { title: 'Baseline', detail: 'build deb + run validated parity verification, compute ground-truth gap set', model: 'sonnet' },
    { title: 'Plan', detail: 'decompose current gaps into granular, monotonic-progress work items (NO file reads)', model: 'sonnet' },
    { title: 'Investigate', detail: 'read-only: derive concrete Bazel edits per work item (no bazel runs, no advisor)', model: 'sonnet' },
    { title: 'Implement', detail: 'sequential edits to the shared tree (Bazel single-workspace lock)', model: 'sonnet' },
    { title: 'Evaluate', detail: 'rebuild deb, re-verify, diff gaps, checkpoint-or-rollback', model: 'sonnet' },
  ],
}

// ---- CRITICAL RULE for all agents -------------------------------------------
// DO NOT call advisor() — it is a server-side tool that blocks for >3 minutes and
// causes the workflow harness to mark the agent as stalled and kill it. If you
// call advisor(), this entire workflow run will fail. Work from the information
// you have been given.
const NO_ADVISOR = 'CRITICAL: Do NOT call advisor(). It blocks for >3 minutes and kills the entire workflow run. Work only from the information provided to you.'

// ---- fixed context -----------------------------------------------------------
const REPO = '/home/bits/go/src/github.com/DataDog/datadog-agent'
const BUILD_CMD =
  'bzl build //packages/agent/linux:debian --platforms=//bazel/platforms:linux_arm64 ' +
  '--repo_env=AGENT_DATA_PLANE_LOCAL_BINARY=' + REPO + '/agent-data-plane-stub ' +
  '--repo_env=MINIMIZED_BTFS_PATH=' + REPO + '/pkg/ebpf/testdata/btfs-compressed'
const VERIFY = 'bash /tmp/verify-parity.sh /tmp/parity'

// Validated verification script spec (recreate at /tmp/verify-parity.sh if missing):
// dpkg-deb --fsys-tarfile <deb> | tar t | norm > deb.txt
// norm < /tmp/ref-tree.txt > ref.txt
// norm = sed 's|^\./||; s|/$||' | grep -v '^$' | grep -vE '\.pyc$|__pycache__'
//       | grep -vE 'embedded/lib/python3\.13/(idlelib|turtledemo|tkinter|lib2to3|test|ensurepip)/'
//       | sed -E 's|(\\.so\\.[0-9]+)\\.[0-9.]+$|\\1|' | sort -u
// comm -13 deb.txt ref.txt > missing.txt (in ref, not in deb)
// comm -23 deb.txt ref.txt > extra.txt
// grep 'agent-data-plane' missing.txt > missing_blocked.txt
// grep -v 'agent-data-plane' missing.txt > missing_actionable.txt
// Prints MISSING_ACTIONABLE and writes /tmp/parity/{deb,ref,missing,missing_actionable,missing_blocked,extra}.txt
const VERIFY_SPEC =
  'A validated verification script is at /tmp/verify-parity.sh. Run `' + VERIFY + '`. ' +
  'It prints MISSING_ACTIONABLE and writes /tmp/parity/missing_actionable.txt. ' +
  'Recreate from the spec in the workflow comment if missing.'

const BUILD_CTX =
  'Repo: ' + REPO + '. Build command: ' + BUILD_CMD + ' (agent-data-plane stub required locally). ' + VERIFY_SPEC

// Seed gap backlog — ground truth is recomputed each round from the deb.
// The Python site-packages gap (dist-info + package files, ~2260 paths) is the dominant
// category and must be decomposed per-package or per-batch.
const SEED_GAPS = [
  'python_dist_info_metadata: Python package metadata (.dist-info files) — 1956 paths. These are the .dist-info directories for ~239 integration packages not yet installed into embedded/lib/python3.13/site-packages/. DECOMPOSE per-package or per-batch of related packages.',
  'python_package_files: Python site-packages package DATA files (botocore JSON/gz service descriptors, kubernetes models, etc.) — 305 paths. Often co-located with dist-info gaps.',
  'licenses: LICENSE and LICENSES/* files — 32 paths. The top-level opt/datadog-agent/LICENSE and LICENSES/ directory with per-dependency license texts.',
  'misc_other: install markers, version manifests, python-scripts, eBPF test objects, run dir symlink — 15 paths.',
  'python_bundled_libs: bundled .so libs inside Python packages (.libs/ subdirs for aerospike, confluent_kafka, cryptography, psycopg_c) — 11 paths.',
  'embedded_bin_tools: jp.py, jsondiff, jsonpatch, jsonpointer, normalizer in embedded/bin/ — 5 paths.',
  'bin_dist_config: config.py, security-agent.yaml, system-probe.yaml, runtime-security.d/default.policy in bin/agent/dist/ — 4 paths.',
  'rtloader_so2: libdatadog-agent-rtloader.so.2 SONAME symlink missing — 1 path.',
  'embedded_sbin: nfsiosat in embedded/sbin/ — 1 path.',
  'agent_data_plane: the real binary 404s without Vault auth — BLOCKED, do not attempt.',
]

// ---- schemas -----------------------------------------------------------------
const GAP = {
  type: 'object', additionalProperties: false,
  properties: {
    id: { type: 'string' },
    category: { type: 'string' },
    count: { type: 'integer' },
    examplePaths: { type: 'array', items: { type: 'string' } },
    blocked: { type: 'boolean' },
  },
  required: ['id', 'category', 'count', 'examplePaths', 'blocked'],
}

const BASELINE_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    debBuilt: { type: 'boolean' },
    verifyRan: { type: 'boolean' },
    missingActionable: { type: 'integer' },
    missingBlocked: { type: 'integer' },
    sanityOk: { type: 'boolean' },
    gaps: { type: 'array', items: GAP },
    notes: { type: 'string' },
  },
  required: ['debBuilt', 'verifyRan', 'missingActionable', 'missingBlocked', 'sanityOk', 'gaps', 'notes'],
}

const PLAN_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    rationale: { type: 'string' },
    workItems: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: {
          id: { type: 'string' },
          gapCategory: { type: 'string' },
          title: { type: 'string' },
          targetFiles: { type: 'array', items: { type: 'string' } },
          approach: { type: 'string' },
          expectedPathsClosed: { type: 'integer' },
          priority: { type: 'integer' },
        },
        required: ['id', 'gapCategory', 'title', 'targetFiles', 'approach', 'expectedPathsClosed', 'priority'],
      },
    },
  },
  required: ['rationale', 'workItems'],
}

const INVESTIGATE_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    workItemId: { type: 'string' },
    feasible: { type: 'boolean' },
    referenceRecipe: { type: 'string' },
    filesToEdit: { type: 'array', items: { type: 'string' } },
    concreteSteps: { type: 'array', items: { type: 'string' } },
    selfCheckBuildTarget: { type: 'string' },
    risks: { type: 'array', items: { type: 'string' } },
  },
  required: ['workItemId', 'feasible', 'referenceRecipe', 'filesToEdit', 'concreteSteps', 'selfCheckBuildTarget', 'risks'],
}

const IMPLEMENT_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    workItemId: { type: 'string' },
    applied: { type: 'boolean' },
    filesChanged: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: { path: { type: 'string' }, created: { type: 'boolean' } },
        required: ['path', 'created'],
      },
    },
    selfCheckPassed: { type: 'boolean' },
    summary: { type: 'string' },
    error: { type: 'string' },
  },
  required: ['workItemId', 'applied', 'filesChanged', 'selfCheckPassed', 'summary', 'error'],
}

const EVAL_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    debBuilt: { type: 'boolean' },
    buildBroke: { type: 'boolean' },
    missingActionable: { type: 'integer' },
    closedPaths: { type: 'integer' },
    closedGaps: { type: 'array', items: { type: 'string' } },
    remainingGaps: { type: 'array', items: GAP },
    newGaps: { type: 'array', items: GAP },
    checkpointed: { type: 'boolean' },
    rolledBack: { type: 'boolean' },
    notes: { type: 'string' },
  },
  required: ['debBuilt', 'buildBroke', 'missingActionable', 'closedPaths', 'closedGaps', 'remainingGaps', 'newGaps', 'checkpointed', 'rolledBack', 'notes'],
}

// ---- prompts -----------------------------------------------------------------
const baselinePrompt =
  NO_ADVISOR + '\n\n' +
  'You are the BASELINE step of a deb-parity loop. ' + BUILD_CTX + '\n\n' +
  'Steps:\n' +
  '1. Check if the deb at bazel-bin/packages/agent/linux/datadog-agent_7.81.0-localbuild-1_arm64.deb is fresh ' +
  '(newer than any file under deps/ or packages/). If stale, rebuild: `cd ' + REPO + ' && ' + BUILD_CMD + '`\n' +
  '2. Run verification: `cd ' + REPO + ' && ' + VERIFY + '`\n' +
  '3. SANITY CHECK: confirm /tmp/parity/missing_actionable.txt does NOT contain plain libcrypto/libssl/libxml2/libxslt/libexslt/libsqlite3 .so entries (hash-versioned .so.3 inside .libs/ subdirs are OK) AND DOES contain libdatadog-agent-rtloader.so.2. Set sanityOk=false if either check fails.\n' +
  '4. Bucket /tmp/parity/missing_actionable.txt into gap categories with counts + 3-5 example paths each.\n' +
  'Expected: ~2330 actionable missing, dominated by python3.13/site-packages. Report real numbers.'

function planPrompt(round, currentGaps, history, prevActionable) {
  return NO_ADVISOR + '\n\n' +
    'You are the PLAN step, round ' + round + '. ' +
    'IMPORTANT: Do NOT read files, run commands, or do any investigation. ' +
    'Your ONLY job is to decompose the gap list below into work items for the Investigate and Implement agents.\n\n' +
    'Current actionable-missing: ' + prevActionable + '.\n' +
    'Current gaps:\n' + JSON.stringify(currentGaps, null, 1) + '\n\n' +
    'Seed gap descriptions (for context):\n- ' + SEED_GAPS.join('\n- ') + '\n\n' +
    'History of prior rounds:\n' + JSON.stringify(history, null, 1) + '\n\n' +
    'Produce 3-8 work items. Rules:\n' +
    '- Do NOT read files. Do NOT run commands. Only decompose the gap data provided.\n' +
    '- DECOMPOSE large categories (esp. python dist-info/package files) into per-package or per-batch items.\n' +
    '- Prefer discrete, high-certainty gaps first: rtloader SONAME, bin_dist_config, embedded_bin_tools, licenses, misc_other.\n' +
    '- Mark agent-data-plane as blocked; do not plan work items for it.\n' +
    '- Do not repeat items shown as infeasible in history without a different approach.\n' +
    '- Each item must name concrete targetFiles (by path) and an expectedPathsClosed estimate.\n' +
    '- For Python wheels/dist-info: name specific packages (e.g. "install botocore wheel") not "all Python packages".\n' +
    'targetFiles for packaging changes typically include: packages/agent/linux/BUILD.bazel, packages/agent/product/BUILD.bazel, packages/agent/dependencies/BUILD.bazel, or a deps/*/​*.BUILD.bazel file.'
}

function investigatePrompt(wi) {
  return NO_ADVISOR + '\n\n' +
    'You are an INVESTIGATE agent (READ-ONLY). ' + BUILD_CTX + '\n\n' +
    'Work item:\n' + JSON.stringify(wi, null, 1) + '\n\n' +
    'Derive the precise Bazel change needed. Rules:\n' +
    '- READ-ONLY: use Read, Grep, Glob, and Bash (grep/find/ls only). Do NOT run bzl/bazel. Do NOT edit files.\n' +
    '- Find how the reference omnibus produces these paths: search omnibus/config/software/ and cite file:line.\n' +
    '- Find the exact Bazel packaging chain: packages/agent/linux/BUILD.bazel -> packages/agent/product/BUILD.bazel -> packages/agent/dependencies/BUILD.bazel -> any deps/*.BUILD.bazel files.\n' +
    '- Name a SMALL self-check build target (never the full deb — e.g. a dep target or pkg_files target).\n' +
    '- If not feasible this round, set feasible=false and explain in risks.\n' +
    'Read bazel/AGENTS.md to understand build conventions before analyzing BUILD files.'
}

function implementPrompt(inv) {
  return NO_ADVISOR + '\n\n' +
    'You are an IMPLEMENT agent. Repo: ' + REPO + '. Build command: ' + BUILD_CMD + '\n\n' +
    'Apply this investigated change to the SHARED working tree:\n' + JSON.stringify(inv, null, 1) + '\n\n' +
    'Rules:\n' +
    '- Read bazel/AGENTS.md conventions BEFORE editing any Bazel file.\n' +
    '- Make the edits. Run `git add -A` after editing so changes are captured by the round snapshot.\n' +
    '- Self-check ONLY the small target from selfCheckBuildTarget (e.g. `bzl build <target>`). Do NOT build the full deb.\n' +
    '- If self-check fails and you cannot fix it quickly, REVERT your edits (`git checkout -- <files>`; rm created files), set applied=false, explain in error.\n' +
    '- Report every file changed with created=true/false.'
}

function evalPrompt(round, prevActionable, changedFiles) {
  return NO_ADVISOR + '\n\n' +
    'You are the EVALUATE step, round ' + round + '. ' + BUILD_CTX + '\n\n' +
    'Previous actionable-missing: ' + prevActionable + '.\n' +
    'Files changed this round (for rollback if needed): ' + JSON.stringify(changedFiles) + '\n\n' +
    'Steps:\n' +
    '1. Rebuild the deb: `cd ' + REPO + ' && ' + BUILD_CMD + '`. If it FAILS, set buildBroke=true.\n' +
    '2. If build OK, run `cd ' + REPO + ' && ' + VERIFY + '`. Read MISSING_ACTIONABLE and /tmp/parity/missing_actionable.txt.\n' +
    '3. Compare to previous (' + prevActionable + '): compute closedPaths, remainingGaps (bucketed with counts+examples), newGaps.\n' +
    '4. CHECKPOINT or ROLLBACK:\n' +
    '   - PROGRESS (build OK AND missingActionable < ' + prevActionable + '): save snapshot with `cd ' + REPO + ' && git stash create > /tmp/parity/lastgood.sha`. Set checkpointed=true.\n' +
    '   - REGRESSION (buildBroke OR missingActionable > ' + prevActionable + '): revert exactly the changed files — for each entry: if created=false run `git checkout -- <path>`, if created=true run `rm -f <path>`. Do NOT use `git clean`. Set rolledBack=true.\n' +
    '   - UNCHANGED (missingActionable == ' + prevActionable + '): neither checkpoint nor rollback.\n' +
    'Return real numbers — this is the gate the loop trusts.'
}

// ---- loop --------------------------------------------------------------------
const MAX_ROUNDS = (args && args.maxRounds) || 1
const FLOOR = (args && typeof args.floor === 'number') ? args.floor : 0

phase('Baseline')
log('Running baseline verification...')
const baseline = await agent(baselinePrompt, { schema: BASELINE_SCHEMA, model: 'sonnet', phase: 'Baseline', label: 'baseline' })
if (!baseline || !baseline.verifyRan) {
  log('Baseline: verification did not run — aborting.')
  return { aborted: true, reason: 'baseline verification did not run', baseline }
}
if (!baseline.sanityOk) {
  log('Baseline: SANITY FAILED — verify-parity.sh is not measuring correctly. Aborting.')
  return { aborted: true, reason: 'verify-parity.sh sanity check failed', baseline }
}
log('Baseline: ' + baseline.missingActionable + ' actionable-missing, ' + baseline.missingBlocked + ' blocked. Sanity OK.')

let currentGaps = baseline.gaps
let prevActionable = baseline.missingActionable
let noProgress = 0
const history = []

for (let round = 1; round <= MAX_ROUNDS; round++) {
  log('=== Round ' + round + '/' + MAX_ROUNDS + ' — ' + prevActionable + ' actionable-missing ===')

  // ---- Plan ----
  phase('Plan')
  const plan = await agent(planPrompt(round, currentGaps, history, prevActionable),
    { schema: PLAN_SCHEMA, model: 'sonnet', phase: 'Plan', label: 'plan:r' + round })
  if (!plan || !plan.workItems || !plan.workItems.length) {
    log('Round ' + round + ': no work items from Plan — stopping.')
    break
  }
  const items = plan.workItems.sort((a, b) => a.priority - b.priority).slice(0, 8)
  log('Round ' + round + ': planned ' + items.length + ' work items.')

  // ---- Investigate (parallel, read-only) ----
  phase('Investigate')
  const invs = (await parallel(items.map((wi) => () =>
    agent(investigatePrompt(wi), { schema: INVESTIGATE_SCHEMA, model: 'sonnet', phase: 'Investigate', label: 'investigate:' + wi.id })
  ))).filter(Boolean).filter((x) => x.feasible)
  log('Round ' + round + ': ' + invs.length + '/' + items.length + ' feasible.')
  if (!invs.length) {
    log('Round ' + round + ': no feasible items — counting as no-progress.')
    noProgress++
    if (noProgress >= 2) { log('Stuck for 2 rounds — stopping.'); break }
    history.push({ round, planned: items.map((i) => i.id), applied: [], result: null })
    continue
  }

  // ---- Implement (sequential — Bazel single-workspace lock) ----
  phase('Implement')
  const impls = []
  for (const inv of invs) {
    const r = await agent(implementPrompt(inv), {
      schema: IMPLEMENT_SCHEMA, model: 'sonnet', phase: 'Implement', label: 'implement:' + inv.workItemId,
    })
    if (r && r.applied) impls.push(r)
  }
  const changedFiles = impls.flatMap((i) => i.filesChanged || [])
  log('Round ' + round + ': applied ' + impls.length + '/' + invs.length + ' changes (' + changedFiles.length + ' files touched).')

  // ---- Evaluate ----
  phase('Evaluate')
  const ev = await agent(evalPrompt(round, prevActionable, changedFiles),
    { schema: EVAL_SCHEMA, model: 'sonnet', phase: 'Evaluate', label: 'evaluate:r' + round })

  const evResult = ev ? {
    closedPaths: ev.closedPaths,
    missingActionable: ev.missingActionable,
    buildBroke: ev.buildBroke,
    rolledBack: ev.rolledBack,
    checkpointed: ev.checkpointed,
  } : null
  history.push({ round, planned: items.map((i) => i.id), applied: impls.map((i) => i.workItemId), result: evResult })

  if (!ev) { log('Round ' + round + ': evaluate returned nothing — stopping.'); break }

  if (ev.buildBroke) {
    log('Round ' + round + ': BUILD BROKE — rolled back. No progress.')
    noProgress++
  } else if (ev.missingActionable > prevActionable) {
    log('Round ' + round + ': REGRESSION ' + prevActionable + ' -> ' + ev.missingActionable + ' — rolled back.')
    noProgress++
  } else if (ev.missingActionable < prevActionable) {
    log('Round ' + round + ': PROGRESS ' + prevActionable + ' -> ' + ev.missingActionable + ' (closed ' + ev.closedPaths + '). Checkpointed.')
    prevActionable = ev.missingActionable
    noProgress = 0
  } else {
    log('Round ' + round + ': no change (' + prevActionable + ' actionable-missing). No progress.')
    noProgress++
  }

  currentGaps = (ev.remainingGaps || []).concat(ev.newGaps || [])

  if (prevActionable <= FLOOR) { log('Reached parity floor (' + FLOOR + '). Done!'); break }
  if (noProgress >= 2) { log('No progress for 2 consecutive rounds — stopping (stuck).'); break }
}

return {
  baselineActionable: baseline.missingActionable,
  finalActionable: prevActionable,
  closedTotal: baseline.missingActionable - prevActionable,
  rounds: history.length,
  history,
}
