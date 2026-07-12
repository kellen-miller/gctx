Both independent passes are in. I've adversarially verified each finding against the plan text and the installed SDK,
dropping or downgrading the ones that don't survive scrutiny (e.g., one pass rated the `--configuration`/ADC issue
CRITICAL claiming the plan holds a "per-config ADC" mental model — but `decision.md:96-97` explicitly states ADC is one
shared global file, so that framing is a strawman; the valid, narrower core is that the flag is decorative). Here is the
consolidated, verified review.

---

# Adversarial review — `gctx` planning packet

**Method.** Two independent adversarial passes (gcloud/ADC/security core; process/CI/interface/intent), each with
filesystem, web, and SDK-source access, plus my own verification against the installed Google Cloud SDK 575.0.1,
`go1.26.5`, the module proxy, the cited Weave repos, and the live repo state. Findings below are deduplicated and
severity-calibrated. The technical core (`gcloud auth login --update-adc` → `set-quota-project` → `activate`) is *
*verified sound** — see "Confirmed correct" at the end — so these are *revise-then-proceed* issues, not *abandon*.

## HIGH

### H1 — Public-repo PII leak vector, and the leak guard is broken in both directions

- **Artifact:** `.gitignore`; `execplan.md:256-259, 398-402`; the untracked `.agent/` tree.
- **Evidence:** `.agent/` is **not** gitignored (`git check-ignore -v .agent/` → no match; repo `.gitignore` contains
  only `.worktrees/`; global `~/.gitignore` doesn't cover it either). Its files contain **11× `kellen@weavelabs.io`**, *
  *14× `chief-discord-bot-502202`**, plus internal names `weavelabs-{dev,prod,common}`, `personal-chief` — and a **913
  KB `plan-review.stream.jsonl`** full-agent transcript. The repo is already **public** (
  `gh api repos/kellen-miller/gctx` → public, default `main`). The plan's own guard,
  `rg -n 'kellen|weavelabs|chief-discord' --glob '!\.agent/**' .`, is doubly defective: (a) it **excludes `.agent/**`
  ** — exactly where the PII lives — so it can't catch that leak; (b) the alternative `kellen` matches the **legitimate
  module slug `kellen-miller`** (in `go.mod`, `README`, build info, every workflow, and `LICENSE` "Kellen Miller"), so
  the guard can **never return zero matches** and the acceptance criterion "expect no matches" is unsatisfiable.
- **Why it matters:** A single `git add -A` publishes a work email, a personal GCP project ID, and an internal reasoning
  transcript to a permanent, indexable public repo — and the one safeguard the plan defines is guaranteed to either miss
  it (false negative on `.agent`) or fire spuriously everywhere (false positive on `kellen-miller`), giving false
  assurance. This is the closest thing to a blocker.
- **Fix / next check:** Add `.agent/` (or the whole planning tree) to `.gitignore` and Milestone 5's ignore list.
  Rewrite the guard to run over tracked files only and to allow the owner slug, e.g.
  `git ls-files -z | xargs -0 rg -n 'weavelabs|chief-discord|\bkellen@' ` with a `kellen-miller`/LICENSE allowlist.
  Decide explicitly whether `.agent/` is ever committed.

### H2 — Rollback reconstructs prior state via a second
`gcloud auth login` (which can itself require a browser) instead of a true ADC backup, and several failure windows leave the wrong identity active

- **Artifact:** `execplan.md:225-254, 430-441`; `decision.md:31-33, 98-101`.
- **Evidence:** Rollback re-derives the prior ADC from the prior **config's declared** account+quota and re-runs
  `gcloud auth login PRIOR_ACCOUNT --update-adc` + `set-quota-project` (`execplan.md:240-241`) — it never backs up the
  actual pre-switch `application_default_credentials.json`. `login.py:89-103 ShouldUseCachedCredentials` falls through
  to the browser flow (`login.py:331`) on any refresh/reauth error, and `GetScopes` adds `REAUTH_SCOPE`, so the rollback
  login can **block on a browser** on the very non-interactive failure path it's meant to cover. Failure windows: if
  target `set-quota-project` fails, ADC is left as **account B with no quota project** (verified: `auth_util.py:410`
  reads `previous_quota_project` from the ADC that `login --update-adc` just wrote *without* a quota via
  `DumpADCToFile()`, so it "restores" `None`); if `activate` fails after both ADC commands succeed, ADC=B while
  config=A. Recovery then depends on printed manual commands.
- **Why it matters:** The packet sells atomic-feeling switching with safe rollback; in reality rollback restores *a*
  config-consistent state (not the user's true prior ADC if it had diverged — which the plan's own Surprises section
  says is the normal case) and can fail to auto-complete, silently leaving you authenticated as the wrong identity.
- **Fix / next check:** Copy `application_default_credentials.json` to a `0600` temp before any mutation and restore it
  byte-for-byte on failure (no second login, no browser dependency, true prior identity+quota preserved). Enumerate each
  failure point's residual state in the plan so tests can assert it.

## MEDIUM

### M1 —
`go get -tool` for golangci-lint/govulncheck/actionlint bloats the distributed module graph, undercutting the "one tiny dependency" story

- **Artifact:** `execplan.md:262-264, 332-334` vs `465-466` ("only runtime Go dependency is `urfave/cli/v3`").
- **Evidence:** golangci-lint v2's own `go.mod` has ~228 direct requires; adding it as a tool dependency grows `go.sum`
  from ~10 to ~928 lines and injects ~200 indirect requires into the root `go.mod`. Every
  `go install github.com/kellen-miller/gctx@latest` consumer must resolve and checksum that graph. The cited exemplar *
  *weave-cli does not do this** — it installs golangci-lint via the pinned `golangci-lint-action` and `go install` in
  CI, keeping `go.mod` clean.
- **Why it matters:** Technically "runtime dep" is still just urfave/cli, but the distributed module a consumer installs
  is ~90× heavier in `go.sum` and graph resolution, diverging from the stated design and the exemplar.
- **Fix / next check:** Use the action-based/`go install`-in-CI approach (as Weave does), or isolate tools in a separate
  `tools/go.mod`, so the installed module stays minimal.

### M2 — The "instant, no-browser switch" premise breaks for reauth/Workspace accounts (including the user's own
`@weavelabs.io`)

- **Artifact:** `execplan.md` Purpose/Milestone 4; `decision.md` Objective.
- **Evidence:** As in H2, cached-credential reuse falls through to a browser flow whenever the target account's session
  needs reauth (2FA/RAPT/SAML), common for Google Workspace domains; the live active account is `kellen@weavelabs.io`.
- **Why it matters:** The headline "no longer remember and sequence auth commands" silently becomes "sometimes opens a
  browser mid-switch." The plan correctly attaches the terminal (`execplan.md:163`) but never sets this expectation.
- **Fix / next check:** Document that a switch may require re-login; consider a pre-flight token probe that warns before
  mutating anything.

### M3 — `--configuration=NAME` on `auth login` and
`set-quota-project` is decorative and misleading (ADC is a single global file)

- **Artifact:** `execplan.md:181-189, 233-234, 440-441`.
- **Evidence:** `set_quota_project.py:Run` calls `AddQuotaProjectToADC(args.quota_project_id)` with no per-config
  parameter; `ADCFilePath()` derives from `CLOUDSDK_CONFIG`/well-known path, never the named config. Verified live:
  `gcloud info … global_config_dir` is unchanged under `CLOUDSDK_ACTIVE_CONFIG_NAME=…`. The flag only changes which
  config's *properties* load during the call — not where ADC is written.
- **Why it matters:** It implies per-config ADC scoping that doesn't exist, misleading maintainers, and is unnecessary (
  both commands take explicit positionals).
- **Fix / next check:** Drop `--configuration=NAME` from both ADC commands; keep it only on `activate`. Tighten
  Objective/Purpose prose (which is otherwise loose but not wrong — `decision.md:96` already states ADC is one shared
  file).

### M4 — A state-file write failure rolls back an *already-successful* switch

- **Artifact:** `execplan.md:213-218` (rename "only after a successful native activation") and `242-244` ("If native
  activation **or state commit** fails after target ADC succeeds, reactivate the prior configuration … and restore its
  ADC").
- **Evidence:** If `activate` succeeds but the atomic rename of the tiny `.gctx-state.json` fails, the plan reverts
  credentials to the prior config — undoing a switch whose ADC and gcloud config both already succeeded, purely because
  a bookkeeping file couldn't be written.
- **Why it matters:** Non-credential failure triggers credential churn and reverts a good switch; worse if the revert
  login then needs a browser (H2).
- **Fix / next check:** Treat state-commit failure as a non-fatal warning (the switch succeeded; only `gctx -` memory is
  lost), or commit previous-state before `activate` since `previous = current` is known up front.

### M5 — Read paths (`--current`, `-`) may not enforce the env-var rejection, so `--current` can misreport

- **Artifact:** `decision.md:61-63`; `execplan.md:188-191` ("before any mutation").
- **Evidence:** Verified live: with `CLOUDSDK_ACTIVE_CONFIG_NAME=doesnotexist123`,
  `gcloud config configurations list --filter=is_active:true --format='value(name)'` returns **empty**. gctx's
  `--current` reads `is_active`; under the env override it would report nothing while the on-disk active config is
  real. "Before any mutation" implies the guard lives on the switch path only.
- **Why it matters:** Breaks the script-friendly `--current` contract and `gctx -`'s "previous" computation under the
  exact env var the plan makes a point of rejecting; fakes won't catch it.
- **Fix / next check:** Reject `GOOGLE_APPLICATION_CREDENTIALS`/`CLOUDSDK_ACTIVE_CONFIG_NAME` as the first step of
  *every* subcommand; add a test.

### M6 — The fake-executable test seam cannot validate the real gcloud contract; "green CI" ≠ "working tool"

- **Artifact:** `decision.md:73-75`; `execplan.md:130-137, 384-395`.
- **Evidence:** Fakes echo expected argv; they cannot verify that real gcloud accepts the flags or that they behave as
  claimed (the `--configuration` no-op, the `None` previous-quota, reauth fall-through, env-var effects all pass against
  a fake). The only real-gcloud check is one manual happy-path smoke test on the author's machine — not in CI, and not
  exercising failure/rollback/reauth.
- **Why it matters:** All required checks can be green while the core value is unproven and flag drift is undetected.
  The release gate ("leave the green PR open") over-trusts CI.
- **Fix / next check:** Add a CI step invoking real `gcloud … --help`/`gcloud info` to detect flag drift; state
  explicitly what the fake seam does NOT validate.

### M7 — `--current` + positional and extra positionals are silently accepted by urfave/cli

- **Artifact:** `execplan.md:154-158, 472-477`.
- **Evidence (cli v3.10.1, empirical):** `gctx --current foo` → `NARG=1, current=true, exit 0`; `gctx a b` →
  `NARG=2, exit 0`. The framework rejects neither; the `Current(ctx) (string,error)` method has no place for a name, so
  precedence is undefined.
- **Why it matters:** Undefined behavior for a script-facing flag; "reject extra positional arguments" must be enforced
  by hand, not assumed from the framework.
- **Fix / next check:** Specify that `--current`+positional and >1 positional are usage errors (exit 2); add parsing
  tests.

### M8 — "Locked tool versions" contradicts `@latest`, breaking plan reproducibility

- **Artifact:** `execplan.md:262-264, 291` ("locked … rather than downloading unversioned tools") vs `332-334` (
  `@latest`).
- **Evidence:** `go get -tool X@latest` resolves the newest version at authoring time; a novice re-running this "
  self-contained" plan (PLANS.md:19) on another day pins different versions (today: golangci-lint v2.12.2, actionlint
  v1.7.12), and golangci-lint behavior changes across minors.
- **Why it matters:** Two runs of the same plan yield different toolchains and possibly different lint pass/fail —
  violating the packet's own reproducibility mandate.
- **Fix / next check:** Pin explicit versions in the `go get -tool` commands.

### M9 — golangci-lint **v2** config schema is unspecified; a v1 `.golangci.yaml` fails CI

- **Artifact:** `execplan.md:259, 332`.
- **Evidence:** golangci-lint v2 requires `version: "2"` and a restructured config (the cited
  `weave-labs/ci/configs/.golangci.yaml` starts with `version: "2"`). A beginner writing a familiar v1 config — which
  the "self-contained for a novice" mandate invites — gets a lint job that errors on parse.
- **Why it matters:** Predictable CI failure the plan should preempt.
- **Fix / next check:** Embed a minimal valid `version: "2"` skeleton in the plan.

## LOW

- **L1 — "Direct and fuzzy switches produce the same native command transcript" is structurally false** (
  `execplan.md:394`): the fuzzy path additionally runs `fzf` (and a list to build rows). Reword to "identical
  *post-selection* `login→set-quota→activate` sub-sequence."
- **L2 — Exit-code/signal plumbing not encoded in the interface** (`execplan.md:154-158, 472-487`): achieving 0/130/1
  needs explicit `signal.NotifyContext` + interrupt classification in `main.go` (as weave-cli does); the
  `(Result,error)`/`ErrSelectionCanceled` abstraction hides it. Specify the `main.go` wiring.
- **L3 — `--version` will print `(devel)` or a `v0.0.0-…` pseudo-version, not `devel`** (`execplan.md:147`):
  `debug.ReadBuildInfo().Main.Version` is `"(devel)"` for local builds and a pseudo-version for `@latest`. Specify the
  normalization and fix the acceptance wording.
- **L4 — Native stderr chatter pollutes the promised clean output** (`execplan.md:163-164, 379-381`): `login.py:100`
  warns "Re-using locally stored credentials…" on every happy-path switch; `_UpdateADC`/set-quota print ADC notes.
  Confirmed **no token/refresh-token leak** on these paths (positive result). Decide stderr handling; assert exact
  stdout shape.
- **L5 — actionlint adds little without shellcheck, and the lint job OS is unspecified** (
  `execplan.md:289-291, 351, 509`): shellcheck ships on `ubuntu-latest`, not `macos-latest`; "locked actionlint
  coverage" overstates. Pin lint to ubuntu or install shellcheck.
- **L6 — `CGO_ENABLED` unspecified while `-race` is required** (`execplan.md:352-353`): defaults work on both hosted
  runners, but `CGO_ENABLED=0` (tempting with `-trimpath`) breaks `-race`. State that the test job keeps CGO on.
- **L7 — Acknowledged/soft items:** `go install …@latest` only works post-merge (plan already notes this at
  `execplan.md:384-388`, and `main` currently has no Go code) and the `go 1.26` directive excludes users on older Go;
  also, validation checks completeness (three fields present) not *coherence* — a complete-but-incoherent config (like
  the live `default`: `@weavelabs.io` account + `chief-discord-bot` project/quota) passes and gets its incoherent
  pairing synced, though gctx does prevent *new* drift via `--no-activate`. Consider softening the "prevents drift"
  framing.

## Confirmed correct (do not "fix")

- `--update-adc`, `--no-activate`, `--brief`, `--configuration` all exist on gcloud 575.0.1; cached-credential reuse is
  browserless (`login.py:279-282`) and `--update-adc` writes a valid `authorized_user` ADC — Google does **not**
  deprecate this path. The core sync mechanism is sound.
- No token/access-token/refresh-token capture on the login/set-quota/activate paths; state file is name-only at `0600`.
- The discovery commands (`gcloud config configurations list --format='json(...)'`, `gcloud info … global_config_dir`)
  work exactly as claimed.
- `urfave/cli/v3@v3.10.1` and `golangci-lint/v2` exist; bare `-` and a config named `current` parse safely (gcloud names
  can't start with `-`).
- The plan **correctly** avoids Weave-only coupling (RunsOn runners, `stitch-auth` GitHub App, `go-private-modules`) —
  the cited `go-test.yaml`/`go-lint.yaml` are indeed RunsOn+private-App reusable workflows unsuitable for a public repo.

---ADVERSARIAL_REVIEW_STATUS---
ISSUES_FOUND: 18
CRITICAL_COUNT: 0
HIGH_COUNT: 2
MEDIUM_COUNT: 9
LOW_COUNT: 7
CONFIDENCE: HIGH
BLOCKING: true
SUMMARY: Core gcloud/ADC mechanism is verified sound, but a public-repo PII leak vector with a doubly-broken guard,
rollback that reconstructs (not backs up) ADC and can block on a browser, and several unsatisfiable/misleading
acceptance gates must be fixed before executing the plan.
---END_ADVERSARIAL_REVIEW_STATUS---
