# Build and publish the initial `gctx` command

This ExecPlan is a living document. The sections `Progress`, `Surprises &
Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to
date as work proceeds. Maintain this document in accordance with
`.agent/PLANS.md` from the repository root.

## Purpose / Big Picture

After this work, a developer with several native Google Cloud CLI
configurations can run `gctx` to fuzzy-select one, run `gctx NAME` to select it
directly, or run `gctx -` to return to the previous one. A successful switch
aligns the active native `gcloud` configuration with the same human account in
Application Default Credentials (ADC) and with that configuration's explicit
quota project. The user no longer has to remember and sequence separate
configuration, login, ADC, and quota commands.

The visible proof is a public `github.com/kellen-miller/gctx` repository, an
installable Go command, behavior tests that simulate native executables, a
manual smoke test against real local configurations, and an open pull request
whose formatting, lint, vulnerability, test, and build checks are green.

The important complexity dividend is that authentication order, validation,
rollback, fuzzy selection, and previous-context state live behind one deep
module. The `urfave/cli` command layer asks for complete operations such as
switching or reporting the current context; it never orchestrates individual
`gcloud` subprocesses.

## Progress

- [x] (2026-07-12 14:54Z) Completed the grill and recorded its decisions in
  `decision.md`.
- [x] (2026-07-12 14:54Z) Created the public repository, established `main` at
  `566642f`, and created the isolated `feat/initial-gctx` worktree.
- [x] (2026-07-12 14:58Z) Copied `.agent/PLANS.md` and drafted this ExecPlan.
- [x] (2026-07-12 15:24Z) Completed adversarial plan review and revised the
  packet to resolve its two blocking findings before implementation.
- [x] (2026-07-12 15:30Z) Activated the implementation goal after the clean
  adversarial re-review.
- [x] (2026-07-12 15:50Z) Implemented command dispatch, native discovery,
  validation, fuzzy/direct/previous/current switching, ADC backup/rollback,
  and state behavior test-first.
- [x] (2026-07-12 16:02Z) Added contributor tasks, public documentation,
  license metadata, pinned tools, and GitHub-hosted PR CI.
- [x] (2026-07-12 16:42Z) Validated fake-executable behavior, all local gates,
  and real switching across work and personal Google accounts. Verified active
  configuration, CLI account/project/quota, ADC quota, `0600` state, and
  `gctx -`; restored `weavelabs-dev` as the active context.
- [x] (2026-07-12 16:10Z) Completed the fresh implementation review, fixed
  subprocess interruption mapping, and reran every local gate successfully.
- [x] (2026-07-12 16:25Z) Completed the adversarial implementation review,
  fixed all four low-severity findings, and reran every local gate successfully.
- [x] (2026-07-12 16:55Z) Committed `03d4c79`, pushed with the explicit remote
  head ref, opened public PR #1, and left it open with all five checks green.

## Surprises & Discoveries

- Observation: the live default configuration had a Weave account paired with
  a personal project and quota project after a native account login, which
  reproduces the cross-context drift `gctx` must prevent.
  Evidence: `gcloud config configurations describe default --format=json`
  reported `core.account=kellen@weavelabs.io` and
  `core.project=billing.quota_project=chief-discord-bot-502202`.
- Observation: native configuration listing can project only the fields `gctx`
  needs, avoiding both the enormous default property tree and one subprocess
  per configuration.
  Evidence: the live command `gcloud config configurations list
  --format='json(name,is_active,properties.core.account,properties.core.project,properties.billing.quota_project)'`
  returned one sparse object per configuration with exactly the requested
  nested properties.
- Observation: `gcloud auth application-default set-quota-project` removes the
  quota value while checking permission but restores the previous value when
  permission is denied. It does not restore the ADC identity written by the
  preceding login step.
  Evidence: the installed SDK's `auth_util.AddQuotaProjectToADC` saves
  `previous_quota_project`, checks `serviceusage.services.use`, and rewrites
  that previous value before raising; identity rollback remains `gctx`'s job.
- Observation: ADC is one file per Cloud SDK configuration directory, not one
  file per named `gcloud` configuration, and `--configuration=NAME` does not
  scope ADC writes.
  Evidence: installed SDK source and live `gcloud info` show the ADC path is
  derived from the global config directory only.
- Observation: local planning and adversarial-review artifacts contain private
  machine evidence and must never be staged in this public repository.
  Evidence: the planning review found the unignored `.agent/` tree contained
  private account/project values and a large raw review transcript.
- Observation: the `--configuration=TARGET` flag does not change the ADC path,
  but it does scope the gcloud properties used by the quota permission-check
  API request. Omitting it can make `set-quota-project TARGET` charge that
  request to a mismatched active configuration's quota project and fail before
  the target can be activated.
  Evidence: the first real switch failed safely with `USER_PROJECT_DENIED` for
  the old active quota project; the equivalent read-only API request succeeded
  under the target configuration and failed under the old one. Installed SDK
  source confirmed the permission check uses ADC credentials while gcloud's
  quota header comes from configuration properties. A transcript regression
  test now requires the target flag on both ADC commands.

## Decision Log

- Decision: use Go 1.26 and `github.com/urfave/cli/v3`.
  Rationale: the user explicitly chose a standalone Go command and this CLI
  framework; the root `main` package keeps `go install` concise.
  Date/Author: 2026-07-12 / user and Codex.
- Decision: treat native named configurations as the only configuration source
  of truth.
  Rationale: a second profile store would duplicate account/project policy and
  create drift. `gctx` validates and switches but never creates or edits.
  Date/Author: 2026-07-12 / user.
- Decision: require explicit `billing/quota_project` and human user accounts.
  Rationale: inferred quota projects and guessed service-account policies would
  violate the tool's correctness promise.
  Date/Author: 2026-07-12 / user.
- Decision: synchronize target ADC before native activation and best-effort
  restore the exact previous ADC file on failure.
  Rationale: native commands do not provide a transaction, so sequencing and
  a byte-for-byte backup avoid a second login and preserve the true prior ADC,
  including cases where it had already diverged from the active configuration.
  Date/Author: 2026-07-12 / user.
- Decision: store previous-context state in the Cloud SDK configuration
  directory returned by `gcloud info`.
  Rationale: state then follows `CLOUDSDK_CONFIG` instead of colliding across
  multiple independent SDK directories.
  Date/Author: 2026-07-12 / Codex, based on live native behavior.
- Decision: reject `GOOGLE_APPLICATION_CREDENTIALS` and
  `CLOUDSDK_ACTIVE_CONFIG_NAME` overrides.
  Rationale: either override would cause the caller's effective context to
  disagree with the state `gctx` updates.
  Date/Author: 2026-07-12 / Codex.
- Decision: support macOS and Linux, external `fzf`, MIT licensing, GitHub-only
  Go installation, and GitHub-hosted PR CI; leave the green PR open.
  Rationale: these are the explicit v1 product and rollout boundaries.
  Date/Author: 2026-07-12 / user.
- Decision: keep `.agent/` local-only and ignored by Git.
  Rationale: planning artifacts contain private live-machine evidence and raw
  review transcripts that do not belong in a public product repository.
  Date/Author: 2026-07-12 / Codex after adversarial review.
- Decision: treat state-file commit failure after successful activation as a
  warning, not a reason to roll back a valid switch.
  Rationale: previous-context bookkeeping must not trigger credential churn or
  undo an otherwise complete context change.
  Date/Author: 2026-07-12 / Codex after adversarial review.

## Outcomes & Retrospective

The initial command is implemented in a lean Go 1.26 module with one runtime
dependency. Direct, fuzzy, current, and previous selection share one deep
manager that validates native configurations, backs up ADC opaquely, orders
login/quota/activation, rolls back failures, and persists `0600` previous
state. Behavior tests cover app parsing, native transcripts, real `os/exec`,
validation, cancellation, ADC restoration/removal, state, and interrupts.

The real smoke test switched from a work context to a personal context and
back with `gctx -`. Active configuration, CLI account/project/quota, ADC quota,
and previous state aligned after each observation; `weavelabs-dev` was restored
as active. The smoke test also found the important gcloud property-scoping
behavior recorded above, and the final transcript regression protects it.

Fresh local verification passed formatting, vet, golangci-lint, govulncheck,
actionlint, race tests, and build. The public repository is at
`https://github.com/kellen-miller/gctx`; PR #1 is open at
`https://github.com/kellen-miller/gctx/pull/1`. Its Ubuntu/macOS build and race
jobs plus lint/vulnerability job are all green. The PR remains unmerged by
design, so `go install github.com/kellen-miller/gctx@latest` becomes usable
after the implementation reaches `main`.

## Context and Orientation

The repository currently contains GitHub's initial `README.md`, an MIT
`LICENSE` and `.gitignore`. The local ignored `.agent` planning tree is not part
of the public repository. There is no Go module or source code yet. The worktree is
`/Users/kellen/development/github/kellen-miller/gctx/.worktrees/initial-gctx`
on `feat/initial-gctx`, based on `main` commit `566642f`, with no upstream until
the branch is pushed.

A Google Cloud CLI configuration is a native named set of properties. The
required properties are `core/account`, `core/project`, and
`billing/quota_project`. ADC is the separate local credential file used by
Google client libraries; the `gcloud` CLI itself does not use ADC. A quota
project attributes quota and billing for client-based APIs and requires the ADC
principal to hold `serviceusage.services.use` on that project.

The root `main.go` will be the installed command. `internal/app` will bind
`urfave/cli` arguments to the use-case interface. `internal/gctx` will be the
deep module that owns native configuration discovery, validation, fuzzy
selection, account/ADC/quota synchronization, activation, rollback, and
previous-state persistence. Split its implementation into focused files only
to improve locality; do not expose each file as another caller-facing concept.

The production implementation invokes `gcloud` and `fzf` through `os/exec`
with argument arrays, never through a shell. Tests place deterministic fake
executables in a temporary `PATH`, allowing the same public command path to be
exercised without real credentials or cloud access. This is the agreed test
seam: tests observe commands, output, state, and exit behavior rather than
reaching into sequencing internals.

## Plan of Work

### Milestone 1: Establish a testable Go command skeleton

Create `go.mod` with module path `github.com/kellen-miller/gctx`, Go 1.26, and
the current v3 release of `github.com/urfave/cli/v3`. Put the executable package
at `main.go` so `go install github.com/kellen-miller/gctx@latest` installs the
`gctx` binary directly. Derive the displayed version from
`runtime/debug.ReadBuildInfo`, using `devel` for local unversioned builds.

Create `internal/app/app.go` with a small use-case interface containing the
complete operations needed by the CLI: select-and-switch, switch-by-name,
switch-to-previous, and current. Construct a `cli.Command` that supports no
positional argument, exactly one configuration name, the special `-` argument,
`--current` (with `-c`), `--help`, and `--version`. Reject extra positional
arguments.
Cancellation from `fzf` exits successfully without changing state; genuine
interrupts use exit code 130; usage errors use exit code 2; validation or
native-command failures use exit code 1 with a concise error on stderr.
`--current` is mutually exclusive with positional arguments, and more than one
positional argument is always a usage error.

The app writes only the stable result summary. The manager suppresses routine
native success output, but it must attach the terminal for `gcloud auth login`
so an expired credential can display its browser URL or prompt. Error output
may include native diagnostics after `gctx`'s contextual message. Never invoke
an access-token command or capture an access token.

Write `internal/app/app_test.go` before the implementation using a fake
use-case adapter. These tests own parsing, dispatch, output, help/version, extra
arguments, cancellation, and exit-code behavior. At the end of this milestone,
`go test ./internal/app` passes while the actual switcher is still absent.

### Milestone 2: Implement native configuration discovery and validation

Create the `internal/gctx` module. Its small caller-facing interface should be
represented by a `Manager` exposing complete operations, not individual
subprocess steps. Keep a private command-runner seam that can stream stdin,
stdout, and stderr for interactive auth commands or capture stdout for JSON and
value queries. Provide the production `os/exec` adapter and a deterministic
fake adapter in tests.

Use one native command to locate and inspect configuration state:

    gcloud config configurations list --format=json(name,is_active,properties.core.account,properties.core.project,properties.billing.quota_project)
    gcloud info --format=value(config.paths.global_config_dir)

Decode only the projected fields required by the tool and identify the current
configuration from `is_active`. Validation must
reject missing account, project, or explicit quota project and print exact
native repair commands scoped with `--configuration=NAME`. Reject accounts
that are not email-like human principals or that end in
`.gserviceaccount.com`. Reject non-empty `GOOGLE_APPLICATION_CREDENTIALS` and
`CLOUDSDK_ACTIVE_CONFIG_NAME` as the first step of every operation, including
`--current` and `-`. Detect missing `gcloud` early and report its prerequisite.

Create table-driven tests for configuration not found, every missing property,
unsupported principals, environment overrides, sparse JSON parsing, malformed
native output, and command failures. The tests must assert actionable errors
without embedding any real account or project from the user's machine.

### Milestone 3: Add fuzzy selection and previous-context state

For no-argument use, turn the projected native configuration list into `fzf`
rows containing name, account, project, and quota columns. Mark incomplete
entries visibly rather than hiding them; selecting one sends it through the
same strict validation path as direct selection. Invoke external `fzf` with
stdin/stdout attached appropriately and parse the selected configuration from
the first tab-separated field. Treat fzf exit 1 (no match) and 130 (cancel or
interrupt) as no-op cancellation when no selection was emitted; other nonzero
codes are errors. A missing `fzf` blocks only interactive selection and
suggests its native install documentation; direct selection continues to work.

Create a private state type containing the previous configuration name. Resolve
its directory through `gcloud info` and use the fixed filename
`.gctx-state.json`. Stage updates in the same directory with mode `0600`,
`fsync`, and atomically rename only after a successful native activation.
Temporary files use a `.gctx-state-*.tmp` pattern and are removed on every
error path. If staging fails, abort before credential changes. Switching from
A to B writes A as previous; `gctx -` switches B to A and writes B as previous,
so repeated `gctx -` toggles. Selecting the already-current configuration
resynchronizes ADC but does not overwrite previous state.

Tests must cover display rows, selection, cancellation, malformed selections,
missing `fzf`, first switch without state, toggle behavior, same-context
resynchronization, file permissions, atomic replacement, corrupt state, and
state isolation across two fake Cloud SDK directories.

### Milestone 4: Hide synchronization, activation, and rollback sequencing

Implement one manager operation that accepts a validated target and hides the
entire sequence. Capture the current configuration, but do not prevent a valid
target from repairing a currently incomplete context. Stage any next state
before credentials change. Locate
`application_default_credentials.json` beside the global Cloud SDK config
directory and copy it to a same-directory `0600` temporary backup before any
credential mutation. If ADC does not exist, record that fact so rollback
removes an ADC created by the failed switch. Copy bytes only: never parse,
log, or expose credential JSON. Synchronize the target without changing the
current CLI property:

    gcloud auth login ACCOUNT --brief --no-activate --update-adc
    gcloud auth application-default set-quota-project QUOTA

Only after both commands succeed, activate the target:

    gcloud config configurations activate NAME --quiet

Then commit the staged previous-state file. If target login, quota setup, or
native activation fails, restore the ADC backup atomically (or remove the new
ADC when no prior file existed) and leave CLI activation/state untouched.
Because activation is last, a failed activation leaves the previous native
configuration active. Join primary and rollback errors so neither is hidden,
and identify the ADC path when automatic restoration cannot complete without
printing credential content. If activation succeeds but state commit fails,
leave the completed switch in place, remove temporary files, and return success
with a warning that `gctx -` history was not updated. On success, securely
remove the backup. Never print tokens, ADC JSON, or refresh credentials.

Drive this milestone test-first with command transcripts. Cover successful
order, expired-login passthrough, quota denial, target activation failure,
state-commit warning, restoration of existing ADC, removal of newly created
ADC, failed restoration reporting, same-context synchronization, and
interruption. Assert exact stdout, that failure never reports success, that no
credential bytes reach output, and that previous state changes only when its
atomic rename succeeds.

### Milestone 5: Add contributor workflow, public documentation, and CI

Update `.gitignore` for `.agent`, local tool binaries, local binaries,
coverage, and `dist` while preserving the already-ignored `.worktrees`. Add a
local golangci-lint v2 configuration whose first field is `version: "2"`.
Keep the root `go.mod` limited to runtime dependencies. Pin explicit versions
for golangci-lint, govulncheck, and actionlint in contributor tasks and CI,
installing them into an ignored local tool directory rather than adding them to
the distributed module graph. Add
`Taskfile.yaml` targets `fmt`, `lint`, `test`, `build`, and `check`, while
keeping the raw Go commands documented for contributors who do not install
Task. `lint` includes the GitHub Actions workflow check.

Replace the generated `README.md` with public documentation. It must explain
the problem, prerequisites, installation via `go install`, every command,
external `fzf`, human-account and macOS/Linux scope, shared ADC semantics,
environment override rejections, state location, rollback limits, and removal.
Its setup tutorial must use placeholders and only native commands:

    gcloud auth login ACCOUNT
    gcloud config configurations create NAME
    gcloud config set account ACCOUNT --configuration=NAME
    gcloud config set project PROJECT --configuration=NAME
    gcloud config set billing/quota_project QUOTA_PROJECT --configuration=NAME

Explain that the account needs `serviceusage.services.use`, commonly through
`roles/serviceusage.serviceUsageConsumer`, on the quota project. Explain how to
list accounts, projects, organizations and configurations; inspect a
configuration; recover CLI and ADC manually; and verify the selected CLI
properties and ADC quota without exposing credentials. Explain that cached
credentials are usually reused but an expired or policy-gated session can open
the browser again. State that users do not
need to run `gcloud auth application-default login` during setup because a
successful `gctx` switch writes ADC from the cached CLI credential and then
sets its quota project.

Add `.github/workflows/ci.yaml` triggered by pull requests with read-only
contents permission and GitHub-hosted runners. Pin checkout and setup-go actions
to full commit SHAs. Use separate named jobs for format/lint/vulnerability,
tests, and build. Run lint on Ubuntu, where actionlint can use shellcheck. Test
and build on both `ubuntu-latest` and `macos-latest` with the Go version read
from `go.mod`; run the race detector with CGO enabled. CI must use explicit
pinned tool versions, not Weave-only reusable workflows, private credentials,
or RunsOn runners. The fake tests verify orchestration but not the native SDK;
the manual live smoke test is the contract check for gcloud behavior.

### Milestone 6: Validate, review, publish the branch, and observe CI

Run the complete local checks from the worktree. Build a temporary binary and
exercise help/version/current plus fake end-to-end scenarios. Then use native
`gcloud` commands to create or correct the user's four local configurations
outside the repository: `weavelabs-dev`, `weavelabs-prod`,
`weavelabs-common`, and `personal-chief`, each with its known account, project,
and matching explicit quota project. This local setup is smoke-test data only
and must never appear in source, fixtures, snapshots, or README examples.

Install or run the local binary and smoke-test a switch between one Weave
context and `personal-chief`, inspect CLI account/project/quota and the ADC
quota field, then run `gctx -` and verify the prior context is restored. Do not
print access tokens or refresh credentials. Restore the user's preferred
context after testing and record only redacted evidence in this plan.

Run the required fresh review and adversarial implementation review, resolve
verified findings, and rerun checks. Commit only intended paths using
Conventional Commits. Push with:

    git push origin HEAD:refs/heads/feat/initial-gctx

Open a pull request to `main` describing behavior and validation. Watch every
check to completion, diagnose failures from live job logs, fix and push as
needed, and leave the green pull request open for user review. Do not merge it,
create a release, or publish binaries/packages.

## Concrete Steps

Run all implementation commands from
`/Users/kellen/development/github/kellen-miller/gctx/.worktrees/initial-gctx`.
Use test-driven slices rather than generating the whole repository at once.

Initialize and maintain the module with:

    go mod init github.com/kellen-miller/gctx
    go get github.com/urfave/cli/v3@v3.10.1
    go mod tidy

Once project tasks exist, use:

    task fmt
    task lint
    task test
    task build
    task check

The equivalent final raw checks are:

    test -z "$(gofmt -l .)"
    go vet ./...
    .tools/bin/golangci-lint run
    .tools/bin/govulncheck ./...
    .tools/bin/actionlint
    go test -race ./...
    go build -trimpath ./...

Run the command during development with:

    go run . --help
    go run . --version
    go run . --current

Do not run live switching until fake-executable tests, the full local check,
and implementation review pass. Before live testing, inspect configurations
with:

    gcloud config configurations list
    gcloud config configurations describe NAME

Expected successful output is concise and contains the selected configuration,
account, project, and quota project but no tokens. Expected validation failures
name the configuration and provide the precise `gcloud config set` repair
command. Expected `fzf` cancellation produces no context change and no error.
A successful switch should render a stable shape such as:

    Switched to example-dev
    Account: user@example.com
    Project: example-dev
    Quota project: example-dev-quota

`gctx --current` and `gctx -c` print only the current configuration name so
they remain script-friendly.

## Validation and Acceptance

The implementation is accepted only when all of the following are observable.
The repository is public and cloneable. `go install
github.com/kellen-miller/gctx@latest` is a documented installation path after
the implementation reaches `main`; until the PR is merged, build or install the
checked-out source locally with `go install .`.

`go test -race ./...` passes every behavior named in `decision.md` without live
cloud access. Formatting, vet, golangci-lint, govulncheck, actionlint, and build
all exit zero. `go run . --help`, `--version`, and `--current` show stable
public output.
Direct and fuzzy switches produce the same post-selection
login-to-quota-to-activation command sequence in tests.
Failures prove rollback and never update previous state prematurely.

The README contains no private account identifiers or organization-specific
project values. Search tracked public files with:

    git ls-files -z | xargs -0 rg -n 'weavelabs|chief-discord|\bkellen@'

and expect no matches. The public owner slug `kellen-miller` and the license
copyright are intentional and not private configuration. The ignored `.agent/`
tree must not appear in `git ls-files`. The MIT license remains present.
The only persistent tool state is the previous native configuration name beside
the selected Cloud SDK directory, with restrictive permissions.

A real smoke switch aligns these observations:

    gcloud config configurations list --filter=is_active:true --format=value(name)
    gcloud config list --format=json
    jq -r .quota_project_id "$(gcloud info --format=value(config.paths.global_config_dir))/application_default_credentials.json"

The first command equals `gctx --current`, the listed account/project/quota
match the selected configuration, and ADC quota equals its explicit quota
project. Running `gctx -` restores the prior configuration and ADC pairing.

The public pull request remains open and every GitHub Actions job is green.
Live `gh pr checks` output, not merely local workflow inspection, is the final
CI evidence.

## Idempotence and Recovery

All read and validation operations are repeatable. Selecting the already-active
configuration resynchronizes ADC but does not destroy the stored previous
context. Atomic state replacement prevents a partially written JSON file from
becoming authoritative.

If target authentication fails before ADC is written, retry after completing
the browser flow. If quota setup fails, grant the selected human account
`serviceusage.services.use` on the explicit quota project and retry. The
manager restores the exact prior ADC file on any later failure. When that
restoration fails, the error identifies the ADC path and retains the restrictive
temporary backup for manual byte-for-byte recovery, but never prints a
credential value.

If the active configuration becomes wrong, recover with:

    gcloud config configurations activate PREVIOUS_NAME

If ADC identity or quota becomes wrong and no backup remains, recover with:

    gcloud auth login PREVIOUS_ACCOUNT --brief --no-activate --update-adc
    gcloud auth application-default set-quota-project PREVIOUS_QUOTA

The feature worktree can be removed after the PR is merged or abandoned without
affecting the primary checkout. Do not delete local Google Cloud configurations
during testing; correct them with native `gcloud config set` commands.

## Artifacts and Notes

Local-only planning evidence and reviews live under the ignored
`.agent/work/initial-gctx`. Store the
planning adversarial review at
`.agent/work/initial-gctx/adversarial/plan-review.md` and the implementation
review at `.agent/work/initial-gctx/adversarial/implementation-review.md`, with
their stream and auth artifacts beside them.

The local exemplar choices are deliberate. `weave-labs/ci` demonstrates pinned
actions, separate Go lint/test jobs, `govulncheck`, the race detector, and
setup-go. `weave-labs/weave-cli` demonstrates `urfave/cli/v3`, a root command
that maps errors to exit codes, Task-based local checks, and build metadata.
The new public repository must not inherit RunsOn, private GitHub App auth,
private module setup, or Weave-specific central lint downloads.

## Interfaces and Dependencies

The module path is `github.com/kellen-miller/gctx` with Go 1.26. The only
runtime Go dependency is `github.com/urfave/cli/v3`. External executable
dependencies are `gcloud` for all operations and `fzf` only for no-argument
interactive selection.

In `internal/app/app.go`, define an unexported use-case interface shaped around
complete behavior:

    type contexts interface {
        SelectAndSwitch(context.Context) (gctx.Result, error)
        Switch(context.Context, string) (gctx.Result, error)
        SwitchPrevious(context.Context) (gctx.Result, error)
        Current(context.Context) (string, error)
    }

Expose `gctx.ErrSelectionCanceled` as the single domain sentinel the command
layer handles as no-op success. The command layer must not learn subprocess
exit codes, state paths, or authentication order.

In `internal/gctx/manager.go`, expose a `Manager` with the four corresponding
operations. `Result` contains only safe display fields: configuration name,
account, project, and quota project. The module hides configuration parsing,
validation, selection, state staging, auth/ADC/quota order, activation, and
rollback. This is the primary deep module and the test interface.

Keep the subprocess runner interface private to `internal/gctx`; it needs
capture and interactive execution modes with context cancellation. Production
uses `os/exec`, while tests use both a fake adapter and fake executables on
`PATH`. Keep state-file implementation private and test it through manager
behavior plus focused filesystem tests. Do not create exported packages for
single-call wrappers. The implementation performs no direct Google HTTP calls,
collects no telemetry, and never parses or prints credential JSON. It may copy
the ADC file opaquely solely for exact rollback; native `gcloud` remains the
sole owner of credential creation and permission checks.

Revision note (2026-07-12): Initial plan compiled from the completed grill,
local repository evidence, installed Google SDK behavior, and current upstream
documentation. It resolves module shape, command behavior, transaction order,
test seams, documentation, CI, live validation, and PR rollout before coding.

Revision note (2026-07-12, improvement pass 1): Replaced per-configuration
describe calls with one verified projected JSON listing, made cancellation
codes explicit, allowed repair when the current configuration is incomplete,
and corrected the pre-merge install and live inspection commands.

Revision note (2026-07-12, improvement pass 2): Added script-friendly current
output, locked actionlint coverage, neutral output examples, and clarified that
`gctx` creates ADC during switching rather than requiring a separate initial
ADC login.

Revision note (2026-07-12, improvement pass 3): Made the app/manager seam and
cancellation contract explicit, fixed the secure state filename and temporary
file lifecycle, and constrained credential handling and native output without
adding shallow packages.

Revision note (2026-07-12, adversarial resolution): Ignored the private
planning tree, replaced login-based rollback with an opaque byte-for-byte ADC
backup, removed decorative per-configuration flags from ADC commands, kept
developer tools out of the root module graph, defined CLI usage errors and
read-path environment guards, documented reauthentication, and made
post-activation state failure a warning rather than credential rollback.
