// The `crabbox` plugin's OWN CUE schema — the typed plugin_input for the
// `crabbox` check verb, which probes the Crabbox CLI and its optional local
// Node.js/PostgreSQL coordinator: in-venue CLI methods (credential-free
// version/providers/config/doctor; broker-backed leases/usage/events/logs) and
// coordinator HTTP probe methods (health/ready).
//
// SELF-CONTAINED by contract: it references NO base def, so it compiles standalone
// (both `cue exp gengotypes` and the host's load-gate compile) AND splices onto the
// base as `base ++ plugin`.
//
// Single source, used two ways:
//  1. GENERATE ../params/cue_types_gen.go via `cue exp gengotypes`, so the provider
//     decodes plugin_input into a TYPED struct rather than a hand-parsed map.
//  2. VALIDATE authored input AT RUNTIME — served over Describe, spliced by the host,
//     and every authored `crabbox:` step's plugin_input checked against #CrabboxInput.
#CrabboxInput: {
	// method — the operation (also the scalar-sugar primary, so `crabbox: version`
	// desugars to {method: "version"}).
	//
	// In-venue CLI methods (version/providers/config/doctor/leases/usage/events/logs)
	// run inside the venue where the `crabbox` binary is installed; coordinator HTTP
	// probe methods (health/ready) are issued from the charly HOST against the
	// resolved coordinator endpoint. All are read-only — there are no mutating
	// methods in this first cut.
	method: "version" | "providers" | "config" | "doctor" | "health" | "ready" | "leases" | "usage" | "events" | "logs"

	// port — the coordinator port to probe for health/ready. Default 8080 (the
	// crabbox-coordinator candy's published port).
	port?: int & >0 & <65536

	// timeout — the HTTP probe timeout. Default 10s.
	timeout?: string & =~"^[0-9]+s$"

	// json_path — a dotted path into the JSON response; when set, the verb's stdout is
	// the value at that path rather than the whole document, so a `stdout:` matcher can
	// assert one field without pattern-matching a blob.
	json_path?: string @go(JSONPath)

	// run_id — run identifier required by the events/logs methods.
	run_id?: string
}