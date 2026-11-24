#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repository_root}"

make generate
if ! git diff --exit-code -- provisioner/config.hcl2spec.go .web-docs; then
  echo "ERROR: generated files are out of date; run 'make generate' and commit the result." >&2
  exit 1
fi

if [[ "${GITHUB_REF:-}" == refs/tags/v* ]]; then
  make release
else
  make release-snapshot
fi
