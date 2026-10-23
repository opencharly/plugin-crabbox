package crabbox

import (
	"testing"
)

// TestResolveHTTPProbe pins the coordinator HTTP probe method → endpoint mapping.
func TestResolveHTTPProbe(t *testing.T) {
	cases := []struct {
		method string
		path   string
	}{
		{"health", "/v1/health"},
		{"ready", "/v1/ready"},
	}
	for _, c := range cases {
		probe, err := resolveHTTPProbe(c.method)
		if err != nil {
			t.Fatalf("resolveHTTPProbe(%q): %v", c.method, err)
		}
		if probe.Path != c.path {
			t.Errorf("resolveHTTPProbe(%q).Path = %q, want %q", c.method, probe.Path, c.path)
		}
	}
	if _, err := resolveHTTPProbe("bogus"); err == nil {
		t.Error("resolveHTTPProbe(bogus) = nil error, want error")
	}
}

// TestResolveCLICall pins the CLI method → argv mapping.
func TestResolveCLICall(t *testing.T) {
	cases := []struct {
		method string
		args   []string
		runID  string
		// provider is the input's provider selection; accepted only by the methods whose
		// CLI command defines `--provider` (measured on the pinned v0.79.0: doctor, leases).
		provider string
		// target and fc are the firecracker-config surface: `--target <os>` and `--firecracker-<key> <v>`.
		// They follow the SAME measured allowlist as provider, because they are the same method's flags.
		target string
		fc     map[string]any
		// cd is the containerd-config surface, passed through on the same allowlist.
		cd map[string]any
	}{
		{"version", []string{"--version"}, "", "", "", nil, nil},
		{"providers", []string{"providers"}, "", "", "", nil, nil},
		{"describe", []string{"providers", "describe", "containerd", "--json"}, "", "containerd", "", nil, nil},
		{"config", []string{"config", "show"}, "", "", "", nil, nil},
		{"doctor", []string{"doctor"}, "", "", "", nil, nil},
		{"leases", []string{"list"}, "", "", "", nil, nil},
		{"usage", []string{"usage"}, "", "", "", nil, nil},
		{"events", []string{"events", "run_abc"}, "run_abc", "", "", nil, nil},
		{"logs", []string{"logs", "run_abc"}, "run_abc", "", "", nil, nil},
		// The pass-through, and its boundary: doctor and leases take it, and a method whose
		// command does NOT define the flag must not grow it (that would turn a working probe
		// into "flag provided but not defined").
		{"doctor", []string{"doctor", "--provider", "local-container"}, "", "local-container", "", nil, nil},
		{"leases", []string{"list", "--provider", "aws"}, "", "aws", "", nil, nil},
		{"usage", []string{"usage"}, "", "aws", "", nil, nil},
		{"providers", []string{"providers"}, "", "aws", "", nil, nil},
		{"version", []string{"--version"}, "", "aws", "", nil, nil},
		// The firecracker-config surface, and its boundary: doctor takes --target and
		// --firecracker-*; a method that does NOT define them must not grow them.
		{"doctor", []string{"doctor", "--target", "linux"}, "", "", "linux", nil, nil},
		{"doctor", []string{"doctor", "--firecracker-binary", "firecracker"}, "", "", "",
			map[string]any{"binary": "firecracker"}, nil},
		{"providers", []string{"providers"}, "", "", "linux", map[string]any{"binary": "firecracker"}, nil},
		// The containerd-config surface: doctor receives --containerd-image; `providers`
		// and `describe` must not grow it.
		{"doctor", []string{"doctor", "--containerd-image", "ghcr.io/opencharly/pod-crabbox:2026.293.1655"}, "", "", "", nil,
			map[string]any{"image": "ghcr.io/opencharly/pod-crabbox:2026.293.1655"}},
		{"providers", []string{"providers"}, "", "", "", nil, map[string]any{"image": "ghcr.io/x/y:1"}},
		{"describe", []string{"providers", "describe", "containerd", "--json"}, "", "containerd", "", nil, map[string]any{"image": "ghcr.io/x/y:1"}},
	}
	for _, c := range cases {
		call, err := resolveCLICall(c.method, c.runID, c.provider, c.target, c.fc, c.cd)
		if err != nil {
			t.Fatalf("resolveCLICall(%q): %v", c.method, err)
		}
		if len(call.Args) != len(c.args) {
			t.Errorf("resolveCLICall(%q, provider=%q).Args = %v, want %v", c.method, c.provider, call.Args, c.args)
			continue
		}
		for i := range c.args {
			if call.Args[i] != c.args[i] {
				t.Errorf("resolveCLICall(%q, provider=%q).Args[%d] = %q, want %q", c.method, c.provider, i, call.Args[i], c.args[i])
			}
		}
	}
	if _, err := resolveCLICall("bogus", "", "", "", nil, nil); err == nil {
		t.Error("resolveCLICall(bogus) = nil error, want error")
	}
	if _, err := resolveCLICall("events", "", "", "", nil, nil); err == nil {
		t.Error("resolveCLICall(events, no run_id) = nil error, want error")
	}
	if _, err := resolveCLICall("describe", "", "", "", nil, nil); err == nil {
		t.Error("resolveCLICall(describe, no provider) = nil error, want error")
	}
}

// TestAcceptsProvider pins the boundary the CLI itself defines, so the input can never be
// attached where it is an argument error. Measured against the pinned v0.79.0:
//
//	crabbox doctor --provider aws     -> ok provider-selection provider=aws source=flag selected=true
//	crabbox list   --provider aws     -> the inventory table
//	crabbox usage  --provider aws     -> flag provided but not defined: -provider
//	crabbox providers --provider aws  -> flag provided but not defined: -provider
func TestAcceptsProvider(t *testing.T) {
	want := map[string]bool{"doctor": true, "leases": true}
	for _, m := range []string{"version", "providers", "describe", "config", "doctor", "leases", "usage", "events", "logs"} {
		if got := acceptsProvider(m); got != want[m] {
			t.Errorf("acceptsProvider(%q) = %t, want %t", m, got, want[m])
		}
	}
}

// TestIsCLIMethod pins the HTTP-vs-CLI split.
func TestIsCLIMethod(t *testing.T) {
	for _, m := range []string{"version", "providers", "describe", "config", "doctor", "leases", "usage", "events", "logs", "containerd-preflight"} {
		if !isCLIMethod(m) {
			t.Errorf("isCLIMethod(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"health", "ready"} {
		if isCLIMethod(m) {
			t.Errorf("isCLIMethod(%q) = true, want false", m)
		}
	}
}

// TestContainerdPreflightIdentity pins the ONE place the preflight reads the daemon
// identity from: the open containerd block with the provider's own defaults.
func TestContainerdPreflightIdentity(t *testing.T) {
	if got := containerdString(nil, "address", defaultContainerdAddress); got != defaultContainerdAddress {
		t.Errorf("nil block address = %q, want %q", got, defaultContainerdAddress)
	}
	block := map[string]any{"address": " /run/containerd/containerd.sock ", "namespace": "default", "runtime": "krun", "image": "ghcr.io/x/y:1"}
	cases := []struct {
		key, want string
	}{
		{"address", "/run/containerd/containerd.sock"},
		{"namespace", "default"},
		{"runtime", "krun"},
		{"image", "ghcr.io/x/y:1"},
		{"missing", "fallback"},
		{"empty", "fallback"},
	}
	block["empty"] = "   "
	for _, c := range cases {
		if got := containerdString(block, c.key, "fallback"); got != c.want {
			t.Errorf("containerdString(%q) = %q, want %q", c.key, got, c.want)
		}
	}
	if got := containerdString(nil, "runtime", defaultContainerdRuntime); got != defaultContainerdRuntime {
		t.Errorf("nil block runtime = %q, want %q", got, defaultContainerdRuntime)
	}
}

// TestNerdctlArgv pins the daemon-identity argv, so every probe names the same
// address/namespace and the preflight cannot drift from the provider.
func TestNerdctlArgv(t *testing.T) {
	got := nerdctlArgv("/run/containerd/containerd.sock", "default", "info")
	want := []string{"nerdctl", "--address", "/run/containerd/containerd.sock", "--namespace", "default", "info"}
	if len(got) != len(want) {
		t.Fatalf("nerdctlArgv = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("nerdctlArgv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestFirstLine pins the skip-reason extraction: the first non-empty line, never a
// blank prefix from a tool that starts its diagnostics with a newline.
func TestFirstLine(t *testing.T) {
	if got := firstLine("\n\n  boom: refused \nsecond"); got != "boom: refused" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine("   \n\t\n"); got != "" {
		t.Errorf("firstLine(blank) = %q, want empty", got)
	}
}

// TestExtractJSONPath pins the json_path extraction.
func TestExtractJSONPath(t *testing.T) {
	doc := `{"status":"ok","coordinator":{"version":"0.48.1"}}`
	if got := extractJSONPath(doc, "status"); got != `"ok"` {
		t.Errorf("extractJSONPath(status) = %s, want \"ok\"", got)
	}
	if got := extractJSONPath(doc, "coordinator.version"); got != `"0.48.1"` {
		t.Errorf("extractJSONPath(coordinator.version) = %s, want \"0.48.1\"", got)
	}
	// Missing path falls back to the raw document.
	if got := extractJSONPath(doc, "nope"); got != doc {
		t.Errorf("extractJSONPath(nope) = %s, want raw doc", got)
	}
	// Non-JSON falls back to the raw document.
	if got := extractJSONPath("not json", "status"); got != "not json" {
		t.Errorf("extractJSONPath(non-json) = %s, want raw", got)
	}
	// Empty path returns the document unchanged.
	if got := extractJSONPath(doc, ""); got != doc {
		t.Errorf("extractJSONPath(empty) = %s, want raw doc", got)
	}
}

// TestShellJoin pins the argv quoting.
func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"crabbox", "status", "--all"})
	want := "'crabbox' 'status' '--all'"
	if got != want {
		t.Errorf("shellJoin = %s, want %s", got, want)
	}
}
