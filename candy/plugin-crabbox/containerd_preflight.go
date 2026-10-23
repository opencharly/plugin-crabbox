package crabbox

import (
	"context"
	"fmt"
	"strings"

	"github.com/opencharly/plugin-crabbox/candy/plugin-crabbox/params"
	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/kit"
	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

// containerd_preflight.go — the `containerd-preflight` method.
//
// W6's e2e bed must prove a containerd lease and then the native
// checkpoint→fork→restore cycle, and each of those legs depends on a resource that
// may legitimately be absent: a CLI built before the W2 provider work, a venue
// with no running containerd, a kernel without `xt_multiport` (so the CNI portmap
// hook cannot publish the lease port), a lease image no repository has published,
// and a checkpoint seam that is not merged upstream. R8 is explicit — a boundary
// that crosses a real service runs against it, and an absent dependency SKIPS
// VISIBLY rather than being mocked. The check framework's per-step SKIP is the
// plugin verdict `skip`, so the ONE place that decides "this leg cannot run, and
// here is exactly what is missing" is this read-only preflight: it runs the real
// probes in the venue and returns either a pass or a skip whose message NAMES the
// absent dependency (the provider, the daemon address, the kernel extension with
// its issue, the exact image ref).
//
// It creates, mutates and releases NO lease. Its only write is the local image
// store: when the configured lease image is not yet present, `nerdctl pull`
// verifies the ref against the real registry instead of guessing from a local
// cache — a pull, never a lease.

const (
	defaultContainerdAddress   = "/run/containerd/containerd.sock"
	defaultContainerdNamespace = "default"
	defaultContainerdRuntime   = "crun"
)

// containerdString reads one key out of the open `containerd:` input block.
func containerdString(block map[string]any, key, def string) string {
	if block == nil {
		return def
	}
	v, ok := block[key]
	if !ok || v == nil {
		return def
	}
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" {
		return def
	}
	return s
}

// firstLine returns the first non-empty line of a tool's stderr for a skip reason,
// so a skip names WHY the probe failed without pasting a wall of output.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// nerdctlArgv builds a nerdctl argv against the configured daemon identity. ONE
// definition, so the daemon address and namespace are spelled the same way at every
// probe (R2).
func nerdctlArgv(address, namespace string, args ...string) []string {
	return append([]string{"nerdctl", "--address", address, "--namespace", namespace}, args...)
}

// invokeContainerdPreflight runs the read-only dependency probe and returns a pass
// (with the observed identity) or a visible skip naming the first absent dependency.
func invokeContainerdPreflight(ctx context.Context, cc kit.CheckContext, op *spec.Op, in *params.CrabboxInput) (*pb.InvokeReply, error) {
	exec := func(script string) (string, string, int, error) {
		return cc.Exec().RunCapture(ctx, script)
	}
	skip := func(format string, a ...any) (*pb.InvokeReply, error) {
		return sdk.ResultJSON("skip", "containerd-preflight: "+fmt.Sprintf(format, a...))
	}
	fail := func(format string, a ...any) (*pb.InvokeReply, error) {
		return sdk.ResultJSON("fail", "containerd-preflight: "+fmt.Sprintf(format, a...))
	}

	address := containerdString(in.Containerd, "address", defaultContainerdAddress)
	namespace := containerdString(in.Containerd, "namespace", defaultContainerdNamespace)
	runtime := containerdString(in.Containerd, "runtime", defaultContainerdRuntime)
	image := containerdString(in.Containerd, "image", "")

	// 1. The CLI must be the source build. `providers` is credential-free; the
	// containerd provider exists only in opencharly/crabbox, never in a release
	// archive of the upstream fork base.
	versionOut, versionErr, versionExit, err := exec(shellJoin([]string{"crabbox", "--version"}))
	if err != nil {
		return fail("crabbox --version: %v", err)
	}
	version := strings.TrimSpace(versionOut)
	if versionExit != 0 {
		return fail("crabbox --version exited %d: %s", versionExit, firstLine(versionErr))
	}
	providersOut, providersErr, providersExit, err := exec(shellJoin([]string{"crabbox", "providers"}))
	if err != nil {
		return fail("crabbox providers: %v", err)
	}
	if providersExit != 0 {
		return fail("crabbox providers exited %d: %s", providersExit, firstLine(providersErr))
	}
	if !strings.Contains(providersOut, "containerd") {
		return skip("the in-venue crabbox CLI (version %s) does not list the containerd provider — it predates the source build of opencharly/crabbox (layer-crabbox must build the pinned commit, not a released archive)", version)
	}

	// 2. A present provider that cannot describe itself is a defect, not a skip.
	describeOut, describeErr, describeExit, err := exec(shellJoin([]string{"crabbox", "providers", "describe", "containerd", "--json"}))
	if err != nil {
		return fail("crabbox providers describe containerd: %v", err)
	}
	if describeExit != 0 {
		return fail("crabbox providers describe containerd exited %d: %s", describeExit, firstLine(describeErr))
	}
	if !strings.Contains(describeOut, `"canonical":"containerd"`) && !strings.Contains(describeOut, `"canonical": "containerd"`) {
		return fail("crabbox providers describe containerd answered without the containerd identity: %s", firstLine(describeOut))
	}

	// 3. The daemon the provider will act on must be reachable. `nerdctl info` is the
	// same client path the provider uses, against the same address and namespace.
	if _, infoErr, infoExit, err := exec(shellJoin(nerdctlArgv(address, namespace, "info"))); err != nil {
		return fail("nerdctl info: %v", err)
	} else if infoExit != 0 {
		return skip("the containerd daemon at %s (namespace %s) is not reachable: %s", address, namespace, firstLine(infoErr))
	}

	// 4. The CNI portmap plugin needs the kernel's multiport match to publish the lease
	// SSH port. Absent, no lease can acquire here (opencharly/layer-containerd#5).
	probe := "if grep -qw multiport /proc/net/ip_tables_matches 2>/dev/null; " +
		"then echo multiport-present; " +
		"elif modprobe -n xt_multiport >/dev/null 2>&1; then echo multiport-present; " +
		"else echo multiport-missing; fi"
	multiportOut, _, _, err := exec(probe)
	if err != nil {
		return fail("xt_multiport probe: %v", err)
	}
	if !strings.Contains(multiportOut, "multiport-present") {
		return skip("the kernel has no xt_multiport match, so the CNI portmap hook cannot publish the lease SSH port (opencharly/layer-containerd#5)")
	}

	// 5. The lease image must exist in a registry the venue can pull from.
	// An image already in the local store is enough for the lease to run, so `inspect`
	// is the fast path; when it is absent, `pull` is the REAL check — the ref is resolved
	// against the registry the lease will use, not assumed from a tag name.
	if image != "" {
		if _, _, inspectExit, err := exec(shellJoin(nerdctlArgv(address, namespace, "image", "inspect", image))); err != nil {
			return fail("nerdctl image inspect: %v", err)
		} else if inspectExit != 0 {
			if _, pullErr, pullExit, err := exec(shellJoin(nerdctlArgv(address, namespace, "pull", "--quiet", image))); err != nil {
				return fail("nerdctl pull: %v", err)
			} else if pullExit != 0 {
				return skip("the lease image %s is not present and cannot be pulled: %s", image, firstLine(pullErr))
			}
		}
	}

	out := fmt.Sprintf("containerd-preflight: ok version=%s address=%s namespace=%s runtime=%s image=%s",
		version, address, namespace, runtime, image)
	if image == "" {
		out = fmt.Sprintf("containerd-preflight: ok version=%s address=%s namespace=%s runtime=%s image=cli-default",
			version, address, namespace, runtime)
	}
	return sdk.VerbVerdict("crabbox", "containerd-preflight", out, nil, op, false)
}
