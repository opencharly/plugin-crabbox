// Package crabbox is the charly plugin serving the `crabbox` check verb — an
// importable root package plus its own go.mod. It probes the Crabbox CLI and its
// optional local Node.js/PostgreSQL coordinator so a candy or box plan can assert
// Crabbox state declaratively:
//
//   - In-venue CLI methods (version/providers/config/doctor/leases/usage/events/logs)
//     run the `crabbox` binary INSIDE the venue over the reverse channel (cc.Exec),
//     where the CLI's config and credentials live.
//   - Coordinator HTTP probe methods (health/ready) hit the coordinator's /v1/health
//     and /v1/ready endpoints from the charly HOST against the resolved endpoint
//     (cc.ResolveEndpoint + cc.HTTPDo).
//
// Why this is not the generic `command:`/`http:` verbs. This verb exists for the two
// things those structurally cannot do:
//
//   - ENDPOINT RESOLUTION. cc.ResolveEndpoint turns the in-venue coordinator port
//     into a host-reachable address, so ONE authored step works unchanged against a
//     pod's published port and a VM's forwarded one.
//   - DOMAIN SEMANTICS. Methods map to the CLI's real surface and return real
//     verdicts, with `json_path:` to assert one field; the in-venue CLI methods run
//     where the `crabbox` binary is installed, which `http:` cannot reach at all.
//
// Dual-placement by construction: the SAME NewProvider()/NewMeta() compile INTO
// charly in-process when listed in compiled_plugins, or cmd/serve serves them
// OUT-OF-PROCESS over go-plugin gRPC when they are not — placement is invisible
// above the provider registry.
package crabbox

import (
	"embed"

	"github.com/opencharly/sdk"
	pb "github.com/opencharly/spec/proto"
)

//go:embed schema/*.cue
var schemaFS embed.FS

// pluginCalVer is this candy's CalVer, advertised over Describe. It must match the
// `version:` in charly.yml — the host reports it when the verb resolves.
const pluginCalVer = "2026.247.0100"

// NewProvider returns the crabbox provider.
func NewProvider() pb.ProviderServer { return &provider{} }

// NewMeta advertises verb:crabbox plus the plugin's self-contained CUE schema (via
// sdk.NewMeta → BuildCapabilities). The verb's whole authoring contract — the method
// enum and every crabbox-exclusive modifier — lives in the served #CrabboxInput
// (schema/crabbox.cue), which the host splices onto the base and validates every
// authored `crabbox:` step's plugin_input against.
//
// Primary is "method", so the scalar sugar `crabbox: version` desugars to
// {method: "version"} the same way the other live probe verbs do.
func NewMeta() pb.PluginMetaServer {
	return sdk.NewMeta(pluginCalVer,
		[]sdk.ProvidedCapability{{
			Class:    "verb",
			Word:     "crabbox",
			InputDef: "#CrabboxInput",
			Primary:  "method",
		}},
		schemaFS)
}
