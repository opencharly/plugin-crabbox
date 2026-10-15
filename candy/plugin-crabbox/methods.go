package crabbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opencharly/plugin-crabbox/candy/plugin-crabbox/params"
	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/kit"
	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

// methods.go is the single table mapping a `crabbox:` method to one probe. Keeping
// it declarative (rather than a switch that also builds requests) means the mapping
// is unit-testable without a live CLI or coordinator, and adding a method is one
// entry.

// httpProbe is a resolved coordinator HTTP probe: the path under the coordinator root.
type httpProbe struct {
	Path string
}

// cliCall is a resolved in-venue CLI invocation.
type cliCall struct {
	Args []string
}

// isCLIMethod reports whether a method runs the `crabbox` CLI inside the venue
// rather than an HTTP probe from the host.
func isCLIMethod(m string) bool {
	switch m {
	case "version", "providers", "config", "doctor", "leases", "usage", "events", "logs":
		return true
	}
	return false
}

// resolveHTTPProbe builds the coordinator HTTP probe for a method.
func resolveHTTPProbe(m string) (httpProbe, error) {
	switch m {
	case "health":
		// Liveness — the coordinator's /v1/health container-probe endpoint.
		return httpProbe{Path: "/v1/health"}, nil
	case "ready":
		// Readiness — the coordinator is accepting lease/run traffic.
		return httpProbe{Path: "/v1/ready"}, nil
	}
	return httpProbe{}, fmt.Errorf("crabbox: unknown HTTP probe method %q", m)
}

// acceptsProvider reports whether the pinned CLI's command for a method defines the
// `--provider <name>` selection flag. MEASURED against the release the org pins
// (v0.78.0 — the artifact layer-crabbox downloads), not inferred:
//
//	$ crabbox doctor --provider aws     -> ok provider-selection provider=aws source=flag selected=true
//	$ crabbox list   --provider aws     -> the ID/NAME/STATE/TYPE/IP inventory table
//	$ crabbox usage  --provider aws     -> flag provided but not defined: -provider
//	$ crabbox providers --provider aws  -> flag provided but not defined: -provider
//
// So the input rides along for doctor and leases and nowhere else: attaching it to a
// command that does not define it turns a working probe into an argument error, which is
// exactly the R4 failure this verb exists to avoid (a documented command that does not work).
func acceptsProvider(m string) bool {
	switch m {
	case "doctor", "leases":
		return true
	}
	return false
}

// resolveCLICall builds the in-venue CLI invocation for a method, appending the provider
// selection where the CLI defines it (see acceptsProvider).
func resolveCLICall(m, runID, provider string) (cliCall, error) {
	args, err := resolveCLIArgs(m, runID)
	if err != nil {
		return cliCall{}, err
	}
	if provider != "" && acceptsProvider(m) {
		args = append(args, "--provider", provider)
	}
	return cliCall{Args: args}, nil
}

// resolveCLIArgs is the method → argv table.
func resolveCLIArgs(m, runID string) ([]string, error) {
	switch m {
	case "version":
		return []string{"--version"}, nil
	case "providers":
		return []string{"providers"}, nil
	case "config":
		return []string{"config", "show"}, nil
	case "doctor":
		return []string{"doctor"}, nil
	case "leases":
		// `crabbox list` is the inventory: at the pinned v0.78.0 it prints an
		// ID/NAME/STATE/TYPE/IP table, and the help lists it as "List Crabbox machines".
		// The per-lease command is `crabbox status <id>` ("Show lease state; add --wait to
		// block until ready"), which has NO `--all` flag — `crabbox status --all` exits with
		// "flag provided but not defined: -all". Verified against the pinned v0.78.0 artifact;
		// the previous provenance line here claimed v0.51.0, twenty-seven releases back.
		return []string{"list"}, nil
	case "usage":
		return []string{"usage"}, nil
	case "events":
		if runID == "" {
			return nil, fmt.Errorf("crabbox: events requires run_id")
		}
		return []string{"events", runID}, nil
	case "logs":
		if runID == "" {
			return nil, fmt.Errorf("crabbox: logs requires run_id")
		}
		return []string{"logs", runID}, nil
	}
	return nil, fmt.Errorf("crabbox: unknown CLI method %q", m)
}

// dispatchHTTP runs one HTTP probe method from the host against the resolved
// coordinator endpoint and returns the body (or the json_path-extracted value) for
// the shared matcher pipeline. A non-2xx status is NOT an error here — the caller's
// stdout matcher asserts the body, and the shared pipeline compares the exit status.
func dispatchHTTP(ctx context.Context, cc kit.CheckContext, addr string, in *params.CrabboxInput) (string, error) {
	probe, err := resolveHTTPProbe(in.Method)
	if err != nil {
		return "", err
	}
	timeout := in.Timeout
	if timeout == "" {
		timeout = "10s"
	}
	req := spec.CheckHTTPRequest{
		Method:  "GET",
		URL:     "http://" + addr + probe.Path,
		Timeout: timeout,
	}
	resp, err := cc.HTTPDo(ctx, req)
	if err != nil {
		return "", fmt.Errorf("crabbox: %s: %v", in.Method, err)
	}
	body := string(resp.Body)
	if in.JSONPath != "" {
		body = extractJSONPath(body, in.JSONPath)
	}
	return body, nil
}

// invokeCLI runs one CLI method inside the venue over the reverse channel and
// returns the shared verdict.
func invokeCLI(ctx context.Context, cc kit.CheckContext, op *spec.Op, in *params.CrabboxInput) (*pb.InvokeReply, error) {
	call, err := resolveCLICall(in.Method, in.RunID, in.Provider)
	if err != nil {
		return sdk.ResultJSON("fail", err.Error())
	}
	args := append([]string{"crabbox"}, call.Args...)
	script := shellJoin(args)
	stdout, stderr, exit, err := cc.Exec().RunCapture(ctx, script)
	if err != nil {
		return sdk.ResultJSON("fail", fmt.Sprintf("crabbox: %s: %v", in.Method, err))
	}
	out := stdout
	if in.JSONPath != "" {
		out = extractJSONPath(stdout, in.JSONPath)
	}
	// The CLI's exit code is the verdict signal; a non-zero exit is reported as a
	// runErr so the shared pipeline maps it to exit=1.
	var runErr error
	if exit != 0 {
		runErr = fmt.Errorf("crabbox %s exited %d: %s", in.Method, exit, strings.TrimSpace(stderr))
	}
	return sdk.VerbVerdict("crabbox", in.Method, out, runErr, op, false)
}

// extractJSONPath pulls a dotted path out of a JSON document (e.g. "status" from
// {"status":"ok"}). Returns the raw document when the path is absent or unparsable.
func extractJSONPath(doc, path string) string {
	if path == "" {
		return doc
	}
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		return doc
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return doc
		}
		cur, ok = m[part]
		if !ok {
			return doc
		}
	}
	b, err := json.Marshal(cur)
	if err != nil {
		return doc
	}
	return string(b)
}

// shellJoin quotes each argument for a single shell invocation.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}
