#!/usr/bin/env bash
set -euo pipefail

fail=0

check() {
	local pkg="$1"; shift
	local deps
	deps=$(go list -test -deps "./internal/upstream/$pkg/..." 2>/dev/null)
	for forbidden in "$@"; do
		if echo "$deps" | grep -q "grokforge/$forbidden"; then
			echo "FAIL: upstream/$pkg imports $forbidden"
			fail=1
		fi
	done
}

check grok internal/flow internal/xai internal/config internal/token internal/upstream/console
check console internal/flow internal/xai internal/config internal/token internal/upstream/grok
check msgutil internal/flow internal/xai internal/config internal/token
check mediautil internal/flow internal/xai internal/config internal/token
check transport internal/flow internal/xai internal/config internal/token internal/upstream/grok internal/upstream/console

[ "$fail" -eq 0 ] && echo "PASS: import boundaries verified"
exit "$fail"
