# plugin-crabbox

The `plugin-crabbox` candy of the [opencharly/charly](https://github.com/opencharly/charly)
plugin library, as a standalone repo (kind-prefixed naming). The candy manifest lives at
`candy/plugin-crabbox/`; the charly resolver fetches this repo at the pinned tag,
go-builds the provider on the host, and serves it out-of-process over go-plugin gRPC.

Serves the `crabbox:` check verb, which probes a
[Crabbox](https://github.com/openclaw/crabbox) CLI and its optional local coordinator:

- **In-venue CLI methods** — `version` / `providers` / `config` / `doctor`
  (credential-free) and `leases` / `usage` / `events` / `logs` (broker-backed,
  requires a `crabbox login` in the venue) — run the `crabbox` binary inside the
  deployment over the reverse channel.
- **Coordinator HTTP probes** — `health` / `ready` against the local
  Node.js/PostgreSQL coordinator's `/v1/health` and `/v1/ready`, host-side via
  endpoint resolution.

Install the CLI itself with the
[`crabbox`](https://github.com/opencharly/layer-crabbox) candy and the coordinator
with the `crabbox-coordinator` candy (opencharly/pod-crabbox).