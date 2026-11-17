#!/usr/bin/env sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
podman_cmd=${PODMAN:-podman}

if [ "$#" -eq 0 ]; then
	printf '%s\n' 'usage: scripts/bbca.sh import|catalog|review|approve [options]' >&2
	exit 2
fi

"$podman_cmd" compose -f "$repo_dir/compose.yaml" build bbca-import
exec "$podman_cmd" compose -f "$repo_dir/compose.yaml" run --rm bbca-import "$@"
