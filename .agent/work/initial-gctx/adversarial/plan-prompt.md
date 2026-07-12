Look at this again with fresh eyes.

You are an adversarial reviewer. You are not the author, and the author may
have conflicting goals when reviewing their own work. Find serious problems in
the planning packet for a new public Google Cloud context-switching CLI.

This is a read-only review. Do not modify files, write files, change external
systems, push branches, create pull requests, or run mutating commands. You may
inspect any relevant paths under $HOME, neighboring repositories, installed
Google Cloud SDK source, official web documentation, browser-accessible pages,
or MCP resources needed to verify or falsify the plan.

Review these artifacts:

- .agent/work/initial-gctx/decision.md
- .agent/work/initial-gctx/execplan.md
- .agent/work/initial-gctx/meta.json
- .agent/PLANS.md
- repository state and initial files under the current worktree

Relevant evidence includes local repositories under
/Users/kellen/development/github/weave-labs/ci and
/Users/kellen/development/github/weave-labs/weave-cli, plus the installed
gcloud CLI. The intended result is a generic, public, Go 1.26 command using
urfave/cli v3. It must dynamically discover native gcloud configurations,
strictly require human account/project/explicit quota properties, synchronize
ADC before activation, roll back failures, support fzf/direct/previous/current
selection, avoid all hardcoded personal values, run public GitHub-hosted PR CI,
and leave a green implementation PR open.

If subagents are available, ask two independent subagents with filesystem read,
web fetch/search, browser, and MCP access to review this work. Tell them that
whoever finds the largest number of serious issues gets five points. If they
are unavailable, run two independent passes yourself.

Do not summarize the work. Challenge it. Report only issues that could change
the plan, implementation, validation, security, or release decision. For each
issue include severity, artifact/path, evidence, why it matters, and the
suggested fix or next check. Call out missing evidence, unsafe credential
handling, incorrect gcloud assumptions, rollback gaps, state races, weak public
interfaces, test blind spots, CI mistakes, or ways the implementation could
satisfy the text but not the user's intent. Do not demand compatibility shims
or scope expansion without an explicit requirement.

End with this exact status block:

---ADVERSARIAL_REVIEW_STATUS---
ISSUES_FOUND: <number>
CRITICAL_COUNT: <number>
HIGH_COUNT: <number>
MEDIUM_COUNT: <number>
LOW_COUNT: <number>
CONFIDENCE: HIGH | MEDIUM | LOW
BLOCKING: true | false
SUMMARY: <one line>
---END_ADVERSARIAL_REVIEW_STATUS---
