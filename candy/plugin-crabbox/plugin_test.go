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
	}{
		{"version", []string{"--version"}, ""},
		{"providers", []string{"providers"}, ""},
		{"config", []string{"config", "show"}, ""},
		{"doctor", []string{"doctor"}, ""},
		{"leases", []string{"status", "--all"}, ""},
		{"usage", []string{"usage"}, ""},
		{"events", []string{"events", "run_abc"}, "run_abc"},
		{"logs", []string{"logs", "run_abc"}, "run_abc"},
	}
	for _, c := range cases {
		call, err := resolveCLICall(c.method, c.runID)
		if err != nil {
			t.Fatalf("resolveCLICall(%q): %v", c.method, err)
		}
		if len(call.Args) != len(c.args) {
			t.Errorf("resolveCLICall(%q).Args = %v, want %v", c.method, call.Args, c.args)
			continue
		}
		for i := range c.args {
			if call.Args[i] != c.args[i] {
				t.Errorf("resolveCLICall(%q).Args[%d] = %q, want %q", c.method, i, call.Args[i], c.args[i])
			}
		}
	}
	if _, err := resolveCLICall("bogus", ""); err == nil {
		t.Error("resolveCLICall(bogus) = nil error, want error")
	}
	if _, err := resolveCLICall("events", ""); err == nil {
		t.Error("resolveCLICall(events, no run_id) = nil error, want error")
	}
}

// TestIsCLIMethod pins the HTTP-vs-CLI split.
func TestIsCLIMethod(t *testing.T) {
	for _, m := range []string{"version", "providers", "config", "doctor", "leases", "usage", "events", "logs"} {
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
