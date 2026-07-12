# gctx

`gctx` switches a native Google Cloud CLI configuration and synchronizes
Application Default Credentials (ADC) to the same human account and explicit
quota project. It feels like `kubectx`, while keeping `gcloud` configurations
as the only source of truth.

```text
$ gctx example-dev
Switched to example-dev
Account: user@example.com
Project: example-project
Quota project: example-quota
```

## Why

Google Cloud supports multiple accounts through named CLI configurations. ADC
is separate from the credentials used by the `gcloud` CLI, though, and there is
only one ADC file per Cloud SDK configuration directory. Changing the active
CLI configuration does not automatically change the identity or quota project
used by local client libraries.

`gctx` performs the complete switch in a safe order:

1. Validate the selected native configuration.
2. Back up the current ADC file without parsing or printing it.
3. Update ADC from the selected account's cached `gcloud` credential.
4. Set the configuration's explicit ADC quota project.
5. Activate the native configuration.
6. Remember the previous configuration for `gctx -`.

If authentication, quota setup, or activation fails, `gctx` restores the exact
previous ADC file and previous native configuration. A login may still open a
browser when cached credentials have expired or an organization requires
reauthentication.

## Requirements

- macOS or Linux
- [Google Cloud CLI](https://cloud.google.com/sdk/docs/install)
- [fzf](https://github.com/junegunn/fzf) for interactive selection
- Go 1.26 or newer when installing from source

`fzf` is optional when using `gctx CONFIGURATION`, `gctx --current`, or
`gctx -`.

## Install

```sh
go install github.com/kellen-miller/gctx@latest
```

Make sure `$(go env GOPATH)/bin` is on `PATH`. Before this initial change is
merged, clone the repository and run `go install .` from the checkout instead.

## Configure gcloud

Authenticate each human account once. The account's organizations and projects
remain governed by Google Cloud IAM; `gctx` does not maintain a separate list.

```sh
gcloud auth login USER_ACCOUNT
gcloud auth list
gcloud organizations list --account=USER_ACCOUNT
gcloud projects list --account=USER_ACCOUNT
```

Create one native configuration for each account/project/quota combination you
want to switch as a unit:

```sh
gcloud config configurations create CONFIGURATION
gcloud config set account USER_ACCOUNT --configuration=CONFIGURATION
gcloud config set project PROJECT_ID --configuration=CONFIGURATION
gcloud config set billing/quota_project QUOTA_PROJECT_ID \
  --configuration=CONFIGURATION
```

Every selectable configuration must explicitly define all three properties:

- `core/account`: a human Google account such as `user@example.com`
- `core/project`: the default project for `gcloud`
- `billing/quota_project`: the consumer project for ADC client-based APIs

`gctx` never infers the quota project from the default project. The account
must have `serviceusage.services.use` on the quota project, commonly through
`roles/serviceusage.serviceUsageConsumer`. An administrator can grant it with:

```sh
gcloud projects add-iam-policy-binding QUOTA_PROJECT_ID \
  --member=user:USER_ACCOUNT \
  --role=roles/serviceusage.serviceUsageConsumer
```

Inspect the resulting native configuration before switching:

```sh
gcloud config configurations list
gcloud config configurations describe CONFIGURATION
```

You do not need to run `gcloud auth application-default login` as a separate
setup step. A successful `gctx` switch uses the selected account's cached CLI
credential to write ADC, then sets its quota project.

## Use

```sh
# Fuzzy-select a native configuration.
gctx

# Switch directly.
gctx CONFIGURATION

# Print only the current configuration name.
gctx --current
gctx -c

# Return to the previous configuration; repeat to toggle.
gctx -
```

Incomplete configurations remain visible in the picker, marked with unset
fields. Selecting one produces the exact native `gcloud config set` command
needed to repair it.

## ADC and environment behavior

ADC is stored at `application_default_credentials.json` under the directory
reported by:

```sh
gcloud info --format='value(config.paths.global_config_dir)'
```

Google client libraries use that file unless another credential source takes
precedence. `gctx` refuses to run when either of these variables is set because
the effective context would not match the files it updates:

- `GOOGLE_APPLICATION_CREDENTIALS`
- `CLOUDSDK_ACTIVE_CONFIG_NAME`

Unset the variable and retry. `gctx` supports human user accounts only in v1;
service accounts, workforce identities, impersonation policy, and credential
files are intentionally outside its scope.

The previous configuration name is stored as `.gctx-state.json` in the same
Cloud SDK directory with mode `0600`. No account tokens or ADC contents are
stored in this state file. Separate `CLOUDSDK_CONFIG` directories therefore
have separate previous-context histories.

## Verify a switch

These commands inspect names and properties without printing access or refresh
tokens:

```sh
gctx --current
gcloud config configurations list \
  --filter='is_active:true' --format='value(name)'
gcloud config list --format=json
jq -r .quota_project_id "$(gcloud info \
  --format='value(config.paths.global_config_dir)')/application_default_credentials.json"
```

The active configuration name should match `gctx --current`; its account,
project, and quota properties should match the selected configuration; and the
ADC quota field should match its explicit `billing/quota_project`.

## Recover manually

`gctx` backs up ADC before changing credentials and restores that backup on
failure. If a process is forcibly terminated or file restoration fails, its
error identifies the restrictive backup path for manual recovery.

To reconstruct a context when no backup remains:

```sh
gcloud config configurations activate PREVIOUS_CONFIGURATION
gcloud auth login PREVIOUS_ACCOUNT --brief --no-activate --update-adc \
  --configuration=PREVIOUS_CONFIGURATION
gcloud auth application-default set-quota-project PREVIOUS_QUOTA_PROJECT \
  --configuration=PREVIOUS_CONFIGURATION
```

Never print or copy the ADC JSON into logs; it contains credentials.

## Develop

The root module contains only runtime dependencies. Pinned developer tools are
installed under the ignored `.tools` directory.

```sh
task check
```

Equivalent core commands are:

```sh
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
go build -trimpath ./...
```

Pull-request CI runs formatting, golangci-lint, govulncheck, actionlint, race
tests, and builds on GitHub-hosted Ubuntu and macOS runners.

## Uninstall

Remove the installed binary and, if desired, the non-credential history file:

```sh
rm "$(go env GOPATH)/bin/gctx"
rm "$(gcloud info \
  --format='value(config.paths.global_config_dir)')/.gctx-state.json"
```

Removing `gctx` does not delete native Google Cloud CLI configurations or ADC.

## License

MIT
