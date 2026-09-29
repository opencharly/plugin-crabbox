# plugin-crabbox

The `crabbox:` check verb for OpenCharly — probe the [Crabbox](https://github.com/openclaw/crabbox)
CLI and its optional local coordinator, served out-of-process (`verb:crabbox`).

This plugin is a standalone Go module the charly loader host-builds and serves
over go-plugin gRPC (`LocalTransport`), so an authored `crabbox:` step dispatches
through the provider registry exactly like a built-in.

## What it provides

| Capability | Surface |
|---|---|
| `verb:crabbox` | the declarative `crabbox:` check step |

## The verb

Three method classes, all read-only:

- **In-venue CLI — credential-free** — `version` (`crabbox --version`),
  `providers`, `config`, `doctor`. Run the `crabbox` binary inside the venue over
  the reverse channel.
- **In-venue CLI — broker-backed** — `leases`, `usage`, `events`, `logs`.
  Require a prior `crabbox login` in the venue.
- **Coordinator HTTP probe** — `health` (`/v1/health`), `ready` (`/v1/ready`),
  issued from the charly host against the resolved coordinator endpoint.

It is not a re-skin of the generic `command:` / `http:` verbs: it adds ENDPOINT
RESOLUTION (`cc.ResolveEndpoint` turns the in-venue coordinator port into a
host-reachable address, so one authored step works unchanged against a pod's
published port and a VM's forwarded one) and DOMAIN SEMANTICS (methods map to the
CLI's real surface and return real verdicts, with `json_path:` to assert one
field; the CLI methods run where the binary is installed, which `http:` cannot
reach).

## How to use it

Compose the plugin candy in a box or check bed's `candy:` list:

```yaml
- '@github.com/opencharly/plugin-crabbox/candy/plugin-crabbox:<tag>'
```

Then author the verb in a plan (scalar sugar shown):

```yaml
- check: the pinned crabbox CLI version is installed
  crabbox: version
  stdout:
    - contains: "0.51.0"
  eventually: 60s
  retry_interval: 5s
```

Install the CLI itself with the `crabbox` candy
([`opencharly/layer-crabbox`](https://github.com/opencharly/layer-crabbox)) and
the coordinator with the `crabbox-coordinator` candy
(`opencharly/pod-crabbox`).

## Layout

- `candy/plugin-crabbox/` — the plugin module: `plugin.go` (the provider +
  `NewProvider()` / `NewMeta()`), `provider.go` (the `Invoke` verdict path),
  `methods.go` (the method dispatch), `schema/crabbox.cue` (the self-contained
  `#CrabboxInput`), `params/cue_types_gen.go`, `plugin_test.go`,
  `cmd/serve/main.go`.
- `candy/plugin-crabbox/charly.yml` — the `plugin-crabbox:` candy entity plus
  the embedded `crabbox-skill:` skill entity.
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-check:crabbox` — the `crabbox:` check verb reference,
  declared by this candy's embedded `crabbox-skill:` skill entity (family
  `check`).
- `/charly-check:check` — the declarative check-step surface the verb is
  authored through.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
