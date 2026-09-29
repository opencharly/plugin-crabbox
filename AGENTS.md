# AGENTS.md — plugin-crabbox

Standalone out-of-tree plugin repo serving the `crabbox:` check verb
(`verb:crabbox`). The plugin is a Go module at `candy/plugin-crabbox/` (module
path `github.com/opencharly/plugin-crabbox/candy/plugin-crabbox`); the root
`charly.yml` only declares `discover: candy` so the repo is a project and its
candy is scanned.

Canonical files:

- `candy/plugin-crabbox/charly.yml` — the `plugin-crabbox:` candy entity
  (`plugin:` block, `plan:` check) plus the embedded `crabbox-skill:` skill
  entity (family `check`, the source for `/charly-check:crabbox`).
- `candy/plugin-crabbox/plugin.go` / `provider.go` — the verb provider and the
  `Invoke` verdict path.
- `candy/plugin-crabbox/methods.go` — the method dispatch (in-venue CLI vs
  coordinator HTTP probe).
- `candy/plugin-crabbox/schema/crabbox.cue` — the self-contained `#CrabboxInput`
  (single source for `params/cue_types_gen.go`).
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-check:crabbox` — the `crabbox:` check verb reference (declared by
  this candy's own `crabbox-skill:` entity). Load before changing the verb
  surface.
- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the `verb` provider class, the per-plugin CUE-schema contract.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-crabbox/` — compile the plugin module.
- `go test ./...` in `candy/plugin-crabbox/` — the plugin's Go tests.
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- The R10 consumers are the crabbox-bearing beds (`check-crabbox-pod`,
  `check-crabbox-e2e-pod`), whose checks compose this plugin alongside the
  `crabbox` CLI candy and the coordinator candy.

## Modify this repo

- Edit the `plugin-crabbox:` candy entity, the Go source, and
  `schema/crabbox.cue` **together** — the schema is the single source for the
  verb's `params/` struct; regenerate `params/cue_types_gen.go` from it.
- Keep the `crabbox-skill:` entity in step with any verb-surface change — it is
  the projected source for `/charly-check:crabbox`.

## Landing

- PR-only. Every change lands through a pull request; the org-required
  `charly/pr-validator` validates the diff and body and arms native auto-merge on
  PASS. Direct pushes to `main` are blocked.
- History lives in `CHANGELOG/` (written by `tag-on-merge` at merge time); the PR
  body IS the changelog.
- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Do not
  restate its rules here.
