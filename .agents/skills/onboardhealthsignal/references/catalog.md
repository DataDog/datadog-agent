# Catalog slice (dd-source, optional enrichment)

Spec of record: `domains/evp-workers/apps/agthealth-worker/src/catalog/README.md` in ddoghq/dd-source, at the commit you checked out. Read it at run time; it is the only reference you may rely on. Do not depend on any particular sample template existing (older commits have a different set): list `src/catalog/templates/` and use the closest existing one only as a layout hint.

The catalog adds display content (title, description, remediation, variants, AI prompt) keyed by `issue_type`. It is ADDITIVE: with the resolver off (most prod orgs) or a thin report, the worker drops reports missing title, description, or severity, and severity is never filled from the catalog. The agent report must be complete first.

## When
- Skip the whole slice while detection is a TODO: a catalog template (and its dd-source PR) for an issue type nothing emits is noise. Hold it until detection exists.
- Otherwise default yes for Path A and B signals once the user confirms (staging has the resolver on for all orgs). Mark it optional for prod visibility. Skip for Path C (v2).
- If a real gate cannot run (steps 3 to 4), mark the slice BLOCKED in the report and do not open the dd-source PR. Never hand-splice generated TypeScript.

## Ordering: overlap it with the agent work (saves about 3 minutes)
The slice needs only the built issue text, not the finished detection code. Start it as soon as the module is rendered and `BuildIssue` compiles:
1. Render the agent module (names, `BuildIssue`).
2. In parallel: (a) finish detection, tests and gazelle in the agent worktree; (b) steps 1 to 3 below in a scratch dd-source worktree.
3. When both are done, run `scripts/verify_all.sh --catalog-dir <dd-source wt> ...` once: its catalog gate runs the real gates (step 4) in parallel with the agent gates.
Regenerate the fixture (step 3) after any later change to `BuildIssue` text, or the byte-for-byte comparison fails.

## Steps
1. Checkout. For a historical snapshot get the commit with `bash <skill-dir>/scripts/preflight.sh catalog-commit <agent-clone> <agent-sha> <dd-source-clone>` (the last commit strictly before the agent commit; do not run `git worktree list` in a shared clone, it lists other runs' paths). Create the worktree WITHOUT a full checkout (a plain `git worktree add` on a blobless clone hangs for minutes fetching every blob):
   `git worktree add --no-checkout --detach <path> <sha> && cd <path> && git sparse-checkout set domains/evp-workers && git checkout`
   (about 5 s when the blobs are local; otherwise it needs working network, use `GIT_SSH_COMMAND="ssh -o BatchMode=yes -o ConnectTimeout=5"` so it fails fast). Sparse set the WHOLE workspace root, not just the app.
2. Write `src/catalog/templates/<issue_type>.md`: front matter (`issue_type`, `issue_name`, `title`, `category`, `location`, `severity`, `vars` mapping context keys with defaults) then description and `### Remediation` steps with `{{placeholders}}`. Omit `severity` if the reporter computes it dynamically.
   The template must reproduce the agent's title, description, summary, and steps CHARACTER FOR CHARACTER from the metadata keys (every `{{placeholder}}` maps to a key the reporter sends in `extra`); the fixture comparison is byte for byte, so any wording drift fails. A placeholder inside quotes (for example `"{{socket_path}}"`) breaks when the value contains a double quote: keep such values quote-free or escape them. A YAML title containing `: ` must be quoted.
3. Generate fixtures from the REAL reporter, never by hand. For a static template (no `variant_key`) write `cases.json` as `[{"context":{...}}]`: no label, so the file is `<issue_type>.md`, the README convention. For a variant template give each case its variant as `"label"` (`<issue_type>.<variant>.md`); `--label X` fills cases that have none. Then
   `python3 -I <skill-dir>/scripts/gen_catalog_fixture.py --spec spec.json --worktree <agent-wt> --cases cases.json --out <dir>`
   (it prints `quoting_warnings` when a metadata value may not round-trip) and copy the `.md` files to `src/catalog/testdata/`. Front matter is the reporter `extra` (every Extra key); the body is the resolved fields. `(none - reason)` marks an intentionally unset field.
4. Real gates (all via node, turbo and `node_modules/.bin`; NEVER `npm install` or `npm ci` here: `npm` in this shell is an alias for `scfw run npm`, the supply-chain firewall may block packages, and the scfw venv may be missing; report a block instead of bypassing it). `scripts/verify_all.sh` runs this for you with `--catalog-dir`; by hand, with `EW=<checkout>/domains/evp-workers` and `APP=$EW/apps/agthealth-worker`:
   ```
   # one-time shim: ONLY so turbo's tsc checks can run on older commits; it is NOT a lint substitute
   mkdir -p /tmp/ewshim && printf '#!/bin/sh\nexit 0\n' > /tmp/ewshim/eslint && chmod +x /tmp/ewshim/eslint
   export PATH=/tmp/ewshim:$EW/node_modules/.bin:$PATH TURBO_CACHE_DIR=$EW/.turbo/cache   # cache path is gitignored
   cd $EW && turbo run build --filter=@event-workers/testkit --concurrency=4      # builds the libs jest needs (cold ~10-13 s)
   cd $APP && node scripts/gen-catalog-templates.mjs && node scripts/gen-catalog-templates.mjs   # twice; idempotent, <1 s
   cd $EW && node_modules/.bin/prettier --check apps/agthealth-worker/src/catalog/generated/*.ts apps/agthealth-worker/src/catalog/index.ts
   cd $APP && node --no-warnings --experimental-vm-modules ../../node_modules/jest/bin/jest.js --no-cache src/catalog
   ```
   Why each step: workspace libs are symlinks whose `dist/` is gitignored and unbuilt, so jest fails with `Cannot find module .../jest-transformer/dist/index.js` until the turbo build runs; older commits run `tsc --noEmit && eslint` in each lib's `check` script and eslint is not in package-lock (CI lints through Bazel), hence the shim; jest needs `--experimental-vm-modules` and must run from `$APP` so the package.json jest preset applies.
   The PR body MUST say: "eslint was not run (a no-op shim let turbo's tsc checks run); CI lints through Bazel".
   turbo adds an agent-rules block to a tracked `AGENTS.md` whenever it detects an agent, even on `--dry`. Do not hide agent identity: after the run, `git checkout -- <the AGENTS.md it touched>` (verify_all.sh does this, also on failure). Set `TURBO_CACHE_DIR` as above so no stray `.turbo/` appears at the git root.
   Generated `generated/*.ts` and `index.ts` MUST be prettier-formatted before commit: `git diff --stat` touches only the new blocks, and `check_signal.py ... --catalog-ts-dir <catalog>/generated` (check 23) passes: object keys are quoted only when hyphenated, so a quoted key without a hyphen is the signature of a hand-spliced file.
   Add the template to the structural test list in `resolver.test.ts` and one fixture-driven resolver test (copy an existing entry); the README says every template needs a fixture.
5. Older-commit scratch worktree and node_modules. Option A (0 s, fine for catalog resolver tests): symlink the main tree's `node_modules` into `<scratch>/domains/evp-workers/node_modules` (pass `--node-modules <dir>` to verify_all.sh). `@event-workers/*` then resolve to the MAIN tree's already-built libs and only the worker's own source comes from the scratch commit. The main tree's libs must already be built; do NOT run turbo in a symlinked scratch tree. Option B (faithful, about 50 s): `cp -Rc <main>/domains/evp-workers/node_modules <scratch>/domains/evp-workers/node_modules` (also `tools/cli/node_modules`), then run the build and tests inside the scratch tree. Use B when the change touches libs.
6. Check 11: compare generated fixtures to the committed ones: `check_signal.py ... --catalog-dir <testdata> --generated-dir <out>`.
7. Draft PR in dd-source via dry-run body file; no push without confirmation.

## Worker staging deploy (gated)
`ddr conductor run --target=staging --release-from-head event-workers-agthealth`. Outward-facing: ask first, name the branch and what it deploys, and note that `ddr` is not installed on this machine, so give the user the exact command to run with `!`.
