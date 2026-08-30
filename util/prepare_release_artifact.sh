#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 3 ]]; then
  echo "usage: $0 ARCHIVE BINARY SIGNATURE_BASENAME" >&2
  exit 2
fi

archive_path="$1"
binary_path="$2"
signature_basename="$3"

archive_dir="$(dirname "$archive_path")"
archive_name="$(basename "$archive_path")"
sbom_name="${archive_name}.sbom.json"

cosign sign-blob \
  --yes \
  --new-bundle-format=false \
  --use-signing-config=false \
  --output-certificate "${archive_dir}/${signature_basename}.pem" \
  --output-signature "${archive_dir}/${signature_basename}.sig" \
  "$binary_path"

syft scan "$binary_path" --output "spdx-json=${archive_dir}/${sbom_name}"
