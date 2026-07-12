# Initial `gctx` Public CLI

## Objective

Create a public Go command named `gctx` that makes a native Google Cloud CLI
configuration the single context users select while synchronizing the active
`gcloud` configuration, its human user account, Application Default
Credentials (ADC), and an explicit quota project. The command must hide the
multi-command authentication sequence behind a `kubectx`-style interface
without inventing a second configuration system.

## Confirmed User Decisions

- Create the public repository `github.com/kellen-miller/gctx` and its local
  checkout at `/Users/kellen/development/github/kellen-miller/gctx`.
- Implement the command in Go 1.26 with `github.com/urfave/cli/v3`.
- Discover native `gcloud` configurations dynamically. Do not hardcode any
  account, project, organization, or configuration name.
- Keep `gctx` a strict switcher. It must not create or edit `gcloud`
  configurations.
- Running `gctx` opens an `fzf` selector. Running
  `gctx <configuration>` switches directly. `gctx --current` prints the
  current configuration, and `gctx -` returns to the previous configuration.
- Require every selectable configuration to have explicit `core/account`,
  `core/project`, and `billing/quota_project` properties. Do not infer the
  quota project from `core/project`.
- Support human Google user accounts only in v1. Do not generate, copy, or
  activate service-account keys, and do not infer an impersonation or
  federation policy.
- Synchronize the one global ADC file before activating the target CLI
  configuration. Before mutation, back up the exact existing ADC file with
  restrictive permissions. If synchronization or activation fails, restore
  that file byte-for-byte (or remove the newly created file when none existed),
  leave the active configuration and previous-context state unchanged, and
  report both the primary and rollback errors when rollback also fails.
- Persist the previous context so `gctx -` behaves like `kubectx -`.
- Officially support macOS and Linux in v1. Avoid deliberate Windows-only
  breakage but do not claim or test Windows support yet.
- Use the MIT license.
- Use GitHub-hosted runners for pull-request CI. CI must check formatting,
  linting/vulnerabilities, tests including the race detector, and a build.
- Do not publish a package to a registry or add an automated release pipeline.
  Document installation from the public Go module with `go install`.
- Push the implementation on a feature branch, open a pull request, observe
  its CI, and leave the green pull request open for user review.

## Agent-Recommended Defaults

- Put the `main` package at the module root so installation is
  `go install github.com/kellen-miller/gctx@latest` rather than requiring a
  `/cmd/gctx` suffix.
- Keep the public command thin. Put switching policy in one deep internal
  module whose interface exposes complete user operations rather than leaking
  individual `gcloud` steps to the CLI layer.
- Use the external `fzf` executable rather than embedding a second fuzzy finder
  or terminal UI. Direct selection remains usable when `fzf` is absent.
- Ask native `gcloud info` for the active Cloud SDK configuration directory,
  store a hidden `gctx` state file beside that directory's configuration data,
  and replace it atomically. This keeps previous-context state isolated when a
  user maintains multiple `CLOUDSDK_CONFIG` directories.
- Treat a non-empty `GOOGLE_APPLICATION_CREDENTIALS` as an error because that
  override would make the ADC file updated by `gctx` ineffective.
- Treat a non-empty `CLOUDSDK_ACTIVE_CONFIG_NAME` as an error because the
  caller's environment would continue to override the configuration activated
  by the child `gcloud` process.
- Validate a human account conservatively as an email-like principal that is
  not a `gserviceaccount.com` address. Reject unsupported principal forms with
  an actionable error rather than guessing.
- Pin third-party GitHub Actions to full commit SHAs and use Ubuntu and macOS
  jobs. The local Weave repositories provide conventions for pinned actions,
  `go test -race`, `golangci-lint`, `govulncheck`, and explicit build checks,
  but Weave-only RunsOn runners and private reusable workflows are not suitable
  for this public personal repository.
- Add table-driven tests at the command's public behavior seam using fake
  `gcloud` and `fzf` executables on `PATH`. This verifies real argument order,
  JSON parsing, cancellation, rollback, and state without calling live cloud
  APIs from CI.
- Keep `.agent/` local-only and ignored. It contains the private planning
  record, live-machine evidence, and review transcripts; none of it belongs in
  the public repository.

## Assumptions

- Users install and authenticate the Google Cloud CLI separately.
- Interactive selection requires `fzf` on `PATH`; direct selection does not.
- Native `gcloud` configuration activation is global to the selected Cloud SDK
  configuration directory, so simultaneous switches remain last-writer-wins.
- A user granting a project as an ADC quota project has
  `serviceusage.services.use` on that project. Native `gcloud` validates the
  permission and supplies the authoritative failure.
- `go install ...@latest` is sufficient distribution for the first public
  version; versioned releases and package-manager formulas are future work.

## Open Questions or User Judgments

None. The grill resolved the public interface, supported identities, platform
scope, failure behavior, license, distribution, CI, and rollout.

## Accepted Risks and Failure Modes

- ADC is one shared local credential file. A successful switch intentionally
  replaces its active identity and quota project.
- Native commands cannot provide a true transaction across CLI configuration
  and ADC. `gctx` minimizes partial state by validating first, backing up ADC,
  activating last, and restoring the exact backup on failure. An interrupted
  process or failed file restoration can still require manual recovery.
- Previous-context state is a small file scoped to the native Cloud SDK
  configuration directory. Concurrent invocations may
  overwrite one another; this matches native `gcloud` activation semantics and
  does not justify a cross-process locking subsystem in v1.
- If activation succeeds but the previous-context state rename fails, the
  selected configuration and ADC remain active and `gctx` returns success with
  a warning. Bookkeeping failure must not undo a valid credential switch.
- Service accounts, workforce identities, credential-file overrides, Windows,
  embedded fuzzy search, shell completion, binary releases, and package-manager
  installation are intentionally outside v1.

## Validation Expectations

- `go test -race ./...` passes with behavior tests for direct selection, fuzzy
  selection, cancellation, missing properties, unsupported principals,
  `GOOGLE_APPLICATION_CREDENTIALS`, successful synchronization and activation,
  quota failure rollback, rollback failure reporting, `--current`, and `-`.
- `golangci-lint run`, `govulncheck ./...`, `go vet ./...`, `gofmt` checking,
  and `go build ./...` pass locally and in pull-request CI.
- A manual smoke test against the user's real configurations proves that a
  switch aligns `gcloud config list`, ADC quota project, and `gctx --current`,
  and that `gctx -` returns to the prior configuration.
- README examples use placeholders and native `gcloud` commands only. They
  explain authentication, named configuration creation, explicit account,
  project and quota properties, `serviceusage.services.use`, ADC behavior,
  installation, prerequisites, commands, failure recovery, and limitations.
- The public GitHub pull request is open and every required check is green.

## Source Notes

This decision record compiles the current Codex conversation on 2026-07-12.
Repository evidence came from the local `weave-labs/ci`, `weave-labs/syngine`,
`weave-labs/weave-python`, `weave-labs/weave-go`, and `weave-labs/weave-cli`
checkouts. Current upstream evidence came from Google Cloud CLI/ADC
documentation, the Go 1.26 release documentation, GitHub Actions setup-go
documentation, the official `fzf` repository, and the `urfave/cli` v3 guide.

The worktree is
`/Users/kellen/development/github/kellen-miller/gctx/.worktrees/initial-gctx`
on branch `feat/initial-gctx`, based on `main` commit `566642f`. The branch has
no upstream until it is pushed with an explicit remote head ref.

`CONTEXT.md` and ADRs are intentionally skipped: this is a small greenfield
command with one resolved public interface, and the decision record plus
ExecPlan are sufficient durable context.
