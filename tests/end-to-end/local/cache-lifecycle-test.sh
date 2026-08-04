#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$repository_root"

# The fixture uses only t.TempDir storage plus deterministic session and
# metadata-only filesystem fakes. It never inspects or mutates a real cache.
go test ./internal/daemon -run '^TestCacheLifecycleFixture$' -count=1
