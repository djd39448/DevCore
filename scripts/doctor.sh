#!/usr/bin/env bash
#
# scripts/doctor.sh — DevCore environment health check.
#
# Depends on: the host tools it probes — go, ollama, claude, ccr — and a
#   running Ollama server. With --test-local it also needs claude-code-router
#   running (start it with `ccr start`).
# Depended on by: developers and CI verifying a DevCore environment; this is the
#   Phase 2 exit check from buildspec §9.
# Why it exists: DevCore has several moving parts — the Go toolchain, Ollama,
#   the memory MCP server, and the local-model proxy. This script proves they
#   are present and wired before work begins, so a broken environment surfaces
#   here rather than mid-task.
#
# Exit codes: 0 all checks passed; 1 one or more checks failed; 2 bad arguments.

set -uo pipefail

test_local=false
case "${1:-}" in
  "")           ;;
  --test-local) test_local=true ;;
  --help | -h)
    echo "Usage:"
    echo "  scripts/doctor.sh                check the toolchain and environment"
    echo "  scripts/doctor.sh --test-local   also test the claude -> proxy -> Ollama path"
    exit 0
    ;;
  *)
    echo "doctor: unknown argument '$1' (try --help)" >&2
    exit 2
    ;;
esac

# Run from the repository root so relative paths and `go build` resolve.
cd "$(dirname "$0")/.." || {
  echo "doctor: cannot locate the repository root" >&2
  exit 1
}

fails=0
pass() { printf '  ok    %s\n' "$1"; }
fail() {
  printf '  FAIL  %s\n' "$1"
  fails=$((fails + 1))
}

echo "DevCore doctor — toolchain & environment"

for tool in go ollama claude ccr; do
  if command -v "$tool" >/dev/null 2>&1; then
    pass "$tool is installed"
  else
    fail "$tool is not installed"
  fi
done

tags_file=$(mktemp)
if curl -s --max-time 5 http://localhost:11434/api/tags >"$tags_file" 2>/dev/null; then
  pass "Ollama is running"
  for model in nomic-embed-text llama3.1; do
    if grep -q "$model" "$tags_file"; then
      pass "Ollama model '$model' is available"
    else
      fail "Ollama model '$model' is not pulled (run: ollama pull $model)"
    fi
  done
else
  fail "Ollama is not responding at localhost:11434 (run: ollama serve)"
fi
rm -f "$tags_file"

if go build ./... >/dev/null 2>&1; then
  pass "DevCore builds (go build ./...)"
else
  fail "DevCore does not build (run 'go build ./...' to see why)"
fi

if [ "$test_local" = true ]; then
  echo
  echo "DevCore doctor — local-model path (claude -> claude-code-router -> Ollama)"

  router_code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    http://127.0.0.1:3456/ 2>/dev/null || true)
  if [ "$router_code" = "200" ]; then
    pass "claude-code-router is running on 127.0.0.1:3456"
  else
    fail "claude-code-router is not responding on 127.0.0.1:3456 (run: ccr start)"
  fi

  # Claude Code sends thinking-enabled requests; local models reject them, so
  # MAX_THINKING_TOKENS=0 is required for the round-trip through the proxy.
  reply=$(MAX_THINKING_TOKENS=0 \
    ANTHROPIC_BASE_URL=http://127.0.0.1:3456 \
    ANTHROPIC_API_KEY=dummy ANTHROPIC_AUTH_TOKEN=dummy \
    claude -p "Reply with exactly: ok" 2>/dev/null || true)
  reply=$(printf '%s' "$reply" | tr -d '\n')
  if printf '%s' "$reply" | grep -qi 'ok'; then
    pass "claude -> proxy -> Ollama round-trip works (model replied: '$reply')"
  else
    fail "claude -> proxy -> Ollama round-trip failed (got: '$reply')"
  fi
fi

echo
if [ "$fails" -eq 0 ]; then
  echo "doctor: all checks passed"
  exit 0
fi
echo "doctor: $fails check(s) failed"
exit 1
