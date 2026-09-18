#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "Error: $*" >&2
    exit 1
}

if (( $# < 4 )); then
    fail "Usage: release.sh name build_dir release_dir platform [platform ...]"
fi
name=$1
[[ "$name" =~ ^[a-zA-Z0-9_-]+$ ]] || fail "Invalid executable name."
targets_dir=$(cd -- "$2" && pwd -P)
output=$3
[[ -n "$output" ]] || fail "Release directory must not be empty."
shift 3
platforms=("$@")
archives=()

for platform in "${platforms[@]}"; do
    [[ "$platform" =~ ^[a-z0-9]+-[a-z0-9]+$ ]] || fail "Invalid platform: $platform"
    archive="$name-$platform.zip"
    for previous in "${archives[@]}"; do
        [[ "$previous" != "$archive" ]] || fail "Duplicate platform: $platform"
    done
    archives+=("$archive")
    executable=$name
    [[ "$platform" != windows-* ]] || executable+=.exe
    binary="$targets_dir/$platform/$executable"
    [[ -f "$binary" && -s "$binary" && ! -L "$binary" ]] || fail "Missing or invalid executable: $binary"
done

for tool in zip unzip sha256sum mktemp; do
    command -v "$tool" >/dev/null || fail "Required tool is unavailable: $tool"
done

# Never mix archives from an earlier build into this release or overwrite them.
[[ ! -L "$output" ]] || fail "Release directory must not be a symbolic link."
if [[ -e "$output" ]]; then
    [[ -d "$output" ]] || fail "Release path is not a directory."
    shopt -s nullglob dotglob
    existing=("$output"/*)
    (( ${#existing[@]} == 0 )) || fail "Release directory is not empty; use a new or empty directory."
fi
parent=$(dirname -- "$output")
if [[ ! -d "$parent" ]]; then
    mkdir -p -- "$parent"
fi
release_dir="$(cd -- "$parent" && pwd -P)/$(basename -- "$output")"
stage=$(mktemp -d "${release_dir}.tmp.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT

for platform in "${platforms[@]}"; do
    executable=$name
    [[ "$platform" != windows-* ]] || executable+=.exe
    archive="$stage/$name-$platform.zip"
    echo "Packaging $platform..."
    (
        cd -- "$targets_dir/$platform"
        zip -q "$archive" "$executable"
    )
    unzip -tq "$archive"
    contents=$(unzip -Z1 "$archive")
    [[ "$contents" == "$executable" ]] || fail "Unexpected archive contents: $archive"
done

(
    cd -- "$stage"
    sha256sum -- "${archives[@]}" > checksums.txt
    sha256sum --check --strict checksums.txt
)

# Only expose the complete, verified release. Any failure above returns nonzero
# and the EXIT trap removes the staging directory.
if [[ -d "$release_dir" ]]; then
    rmdir -- "$release_dir"
fi
mv -- "$stage" "$release_dir"
trap - EXIT
echo "Release ready: $release_dir"
