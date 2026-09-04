package crabbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/opencharly/plugin-crabbox/candy/plugin-crabbox/params"
	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/kit"
	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

// provider.go is the out-of-process crabbox verb provider. charly's host dispatches
// a `crabbox:` step to it through the registry (ResolveVerb("crabbox") →
// grpcProvider → Provider.Invoke) with the FULL #Op marshaled as params_json and a
// CheckEnv snapshot as env_json. The out-of-process path runs no host-side matcher
// pipeline, so this Invoke OWNS the whole verdict: resolve the coordinator endpoint
// (health/ready) or run the in-venue CLI (everything else), then evaluate the
// stdout/stderr/exit_status matchers through the shared sdk implementation (R3).

// defaultCoordinatorPort is the crabbox-coordinator candy's published port.
const defaultCoordinatorPort = 8080

// crabboxEnv is the plugin-side decode of the CheckEnv the host ships as the
// Operation env for a `crabbox:` step.
type crabboxEnv struct {
	Box  string `json:"box"`
	Mode string `json:"mode"` // "live" | "box"
}

type provider struct{ pb.UnimplementedProviderServer }

// Invoke runs one `crabbox:` operation.
func (provider) Invoke(ctx context.Context, req *pb.InvokeRequest) (*pb.InvokeReply, error) {
	var op spec.Op
	if len(req.GetParamsJson()) > 0 {
		if err := json.Unmarshal(req.GetParamsJson(), &op); err != nil {
			return sdk.ResultJSON("fail", "crabbox: decode op: "+err.Error())
		}
	}
	var in params.CrabboxInput
	kit.DecodeInput(op.PluginInput, &in)
	var env crabboxEnv
	if len(req.GetEnvJson()) > 0 {
		_ = json.Unmarshal(req.GetEnvJson(), &env)
	}
	method := in.Method

	// Live-deployment verb: there is no running Crabbox context during `charly check
	// box` (a disposable build container), so skip rather than fail — the same
	// contract the other live probe verbs follow.
	if env.Mode == "box" {
		return sdk.ResultJSON("skip", fmt.Sprintf(
			"crabbox: %s requires a running deployment (skip under charly check box)", method))
	}

	cc, err := sdk.NewCheckContext(req.GetExecutorBrokerId(), req.GetEnvJson())
	if err != nil {
		return sdk.ResultJSON("fail", fmt.Sprintf("crabbox: %s: %v", method, err))
	}

	// In-venue CLI methods run inside the venue where the `crabbox` binary is
	// installed; coordinator HTTP probe methods run from the host against the
	// resolved endpoint. Branching here keeps the two contracts (argv + exit code vs
	// HTTP status + body) from bleeding into each other.
	if isCLIMethod(method) {
		return invokeCLI(ctx, cc, &op, &in)
	}

	port := int(in.Port)
	if port == 0 {
		port = defaultCoordinatorPort
	}
	addr, err := cc.ResolveEndpoint(ctx, port)
	if err != nil {
		return sdk.ResultJSON("fail", fmt.Sprintf("crabbox: %s: %v", method, err))
	}
	// No live venue (no-box context) → skip, the analogue of the host's empty-box skip.
	if addr == "" {
		return sdk.ResultJSON("skip", fmt.Sprintf(
			"crabbox: %s has no resolved coordinator endpoint (box=%q)", method, env.Box))
	}

	out, runErr := dispatchHTTP(ctx, cc, addr, &in)

	// The shared exit/stdout/stderr verdict pipeline (R3).
	return sdk.VerbVerdict("crabbox", method, out, runErr, &op, false)
}
