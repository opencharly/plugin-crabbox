package crabbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/opencharly/plugin-crabbox/candy/plugin-crabbox/params"
	"github.com/opencharly/spec/spec"
)

// containerd_preflight_test.go — R7 coverage for the dependency gate. Each case
// scripts a REAL venue's command responses (the exact argv the preflight builds)
// and asserts the verdict: a SKIP whose message NAMES the missing dependency, or a
// PASS when every dependency is present. Without the preflight these cases cannot
// exist (the method did not exist), so the test fails on the pre-change tree.

type fakeExec struct {
	run func(script string) (string, string, int, error)
}

func (f fakeExec) RunCapture(_ context.Context, script string) (string, string, int, error) {
	return f.run(script)
}

func (f fakeExec) Kind() string { return "container" }

// fakeCheckContext implements only Exec; the embedded nil interface satisfies the
// rest of spec.CheckContext, which the preflight never calls.
type fakeCheckContext struct {
	spec.CheckContext
	exec spec.CheckExecutor
}

func (f fakeCheckContext) Exec() spec.CheckExecutor { return f.exec }

type scriptRule struct {
	match string
	out   string
	err   string
	exit  int
}

// scriptedVenue returns a CheckContext whose executor answers each command by the
// FIRST rule whose substring matches, and fails loudly on an unscripted command —
// so a preflight that grew an unexpected probe cannot pass silently.
func scriptedVenue(rules []scriptRule) spec.CheckContext {
	return fakeCheckContext{exec: fakeExec{run: func(script string) (string, string, int, error) {
		for _, r := range rules {
			if strings.Contains(script, r.match) {
				return r.out, r.err, r.exit, nil
			}
		}
		return "", "unscripted command: " + script, 127, nil
	}}}
}

// baseRules scripts a venue whose CLI carries the provider. The match strings are
// single argv TOKENS (the preflight shell-quotes every argument), and each nerdctl
// subcommand has its OWN rule because every probe's script also contains the
// binary name.
func baseRules() []scriptRule {
	return []scriptRule{
		{match: "--version", out: "b7b5aaa312fdcd0711b4407935ce4d50117e8155\n"},                         // 0
		{match: "'describe'", out: `{"provider":{"canonical":"containerd"},"kind":"ssh-lease"}` + "\n"}, // 1
		{match: "'providers'", out: "containerd\nlocal-container\nssh\n"},                               // 2
		{match: "'inspect'", out: "[]\n"},                                                               // 3
		{match: "'pull'", out: ""},                                                                      // 4
		{match: "'info'", out: "Server Version: v2\n"},                                                  // 5
		{match: "ip_tables_matches", out: "multiport-present\n"},                                        // 6
	}
}

func decodeVerdict(t *testing.T, reply interface{ GetResultJson() []byte }) (string, string) {
	t.Helper()
	var w struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(reply.GetResultJson(), &w); err != nil {
		t.Fatalf("decode verdict: %v (%s)", err, reply.GetResultJson())
	}
	return w.Status, w.Message
}

func invokePreflight(t *testing.T, cc spec.CheckContext, in *params.CrabboxInput) (string, string) {
	t.Helper()
	reply, err := invokeContainerdPreflight(context.Background(), cc, &spec.Op{}, in)
	if err != nil {
		t.Fatalf("invokeContainerdPreflight: %v", err)
	}
	return decodeVerdict(t, reply)
}

func preflightInput() *params.CrabboxInput {
	return &params.CrabboxInput{
		Method:   "containerd-preflight",
		Provider: "containerd",
		Containerd: map[string]any{
			"runtime": "crun",
			"image":   "ghcr.io/opencharly/pod-crabbox:2026.293.1655",
		},
	}
}

// TestContainerdPreflightAllPresent: every dependency present → PASS, naming the
// observed identity so the log says WHAT it proved.
func TestContainerdPreflightAllPresent(t *testing.T) {
	status, msg := invokePreflight(t, scriptedVenue(baseRules()), preflightInput())
	if status != "pass" {
		t.Fatalf("status = %q, want pass (%s)", status, msg)
	}
	for _, want := range []string{"containerd-preflight: ok", "namespace=default", "runtime=crun", "ghcr.io/opencharly/pod-crabbox:2026.293.1655"} {
		if !strings.Contains(msg, want) {
			t.Errorf("pass message %q does not name %q", msg, want)
		}
	}
}

// TestContainerdPreflightSkipsNameTheDependency is the R8 gate: each absent
// dependency is a SKIP whose message names the exact thing, never a mock and never
// a failure.
func TestContainerdPreflightSkipsNameTheDependency(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func([]scriptRule) []scriptRule
		wantInMsg []string
	}{
		{
			name: "old CLI has no containerd provider",
			mutate: func(r []scriptRule) []scriptRule {
				r[0].out = "0.79.0\n"
				r[2].out = "local-container\nssh\nexternal\n"
				return r
			},
			wantInMsg: []string{"does not list the containerd provider", "0.79.0", "opencharly/crabbox"},
		},
		{
			name: "daemon unreachable",
			mutate: func(r []scriptRule) []scriptRule {
				r[5].exit = 1
				r[5].err = "dial unix /run/containerd/containerd.sock: connect: no such file or directory"
				return r
			},
			wantInMsg: []string{"not reachable", "/run/containerd/containerd.sock", "default"},
		},
		{
			name: "kernel has no xt_multiport",
			mutate: func(r []scriptRule) []scriptRule {
				r[6].out = "multiport-missing\n"
				return r
			},
			wantInMsg: []string{"xt_multiport", "layer-containerd#5"},
		},
		{
			name: "lease image unpublished",
			mutate: func(r []scriptRule) []scriptRule {
				r[3].exit = 1
				r[3].err = "no such image"
				r[4].exit = 1
				r[4].err = "manifest unknown: ghcr.io/opencharly/pod-crabbox:2026.293.1655"
				return r
			},
			wantInMsg: []string{"lease image ghcr.io/opencharly/pod-crabbox:2026.293.1655", "cannot be pulled", "manifest unknown"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, msg := invokePreflight(t, scriptedVenue(c.mutate(baseRules())), preflightInput())
			if status != "skip" {
				t.Fatalf("status = %q, want skip (%s)", status, msg)
			}
			for _, want := range c.wantInMsg {
				if !strings.Contains(msg, want) {
					t.Errorf("skip message %q does not name %q", msg, want)
				}
			}
		})
	}
}

// TestContainerdPreflightDefectFails: a provider that IS present but cannot
// describe itself is a FAIL, not a skip — a skip there would hide a broken CLI.
func TestContainerdPreflightDefectFails(t *testing.T) {
	rules := baseRules()
	rules[1].exit = 1
	rules[1].err = "provider describe: internal error"
	status, msg := invokePreflight(t, scriptedVenue(rules), preflightInput())
	if status != "fail" {
		t.Fatalf("status = %q, want fail (%s)", status, msg)
	}
	if !strings.Contains(msg, "describe containerd exited 1") {
		t.Errorf("fail message %q does not name the failing probe", msg)
	}
}

// TestContainerdPreflightNoImage: with no image configured, the CLI's own default
// is the lease image, so the image probes are skipped and the pass says so.
func TestContainerdPreflightNoImage(t *testing.T) {
	rules := baseRules()
	rules = append(rules[:3], rules[5:]...) // drop the image inspect/pull rules
	in := preflightInput()
	delete(in.Containerd, "image")
	status, msg := invokePreflight(t, scriptedVenue(rules), in)
	if status != "pass" {
		t.Fatalf("status = %q, want pass (%s)", status, msg)
	}
	if !strings.Contains(msg, "image=cli-default") {
		t.Errorf("pass message %q does not report the CLI default image", msg)
	}
}
