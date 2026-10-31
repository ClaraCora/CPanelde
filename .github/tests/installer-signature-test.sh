#!/bin/sh
set -eu

# Only extract download/verification helpers. Never execute installation or services.
installer=${1:-./install.sh}
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT HUP INT TERM
TMP_DIR="${test_root}/staging"
fixture_dir="${test_root}/release"
mkdir -p "$TMP_DIR" "$fixture_dir"
awk '
  /^(component_release_tag|ensure_release_public_key|download_release_file|verify_download|load_signed_manifest|try_download|download_verified)\(\) \{/ { copying = 1 }
  copying { print }
  /^\}/ { copying = 0 }
' "$installer" >"${test_root}/functions.sh"
. "${test_root}/functions.sh"
ensure_release_public_key
# An environment path must never override the installer's compiled trust anchor.
cp "$RELEASE_PUBLIC_KEY_FILE" "${test_root}/compiled-public.pem"
RELEASE_PUBLIC_KEY_FILE=/invalid/operator-environment-override
ensure_release_public_key
cmp "$RELEASE_PUBLIC_KEY_FILE" "${test_root}/compiled-public.pem"
if download_release_file http://127.0.0.1/forbidden "${test_root}/forbidden" 256 2>/dev/null; then
  echo 'plaintext release transport unexpectedly accepted' >&2; exit 1
fi

# Ephemeral fixture key; no production private key is required by tests or PRs.
openssl genpkey -algorithm ED25519 -out "${test_root}/fixture-private.pem"
openssl pkey -in "${test_root}/fixture-private.pem" -pubout -out "${test_root}/fixture-public.pem"
ensure_release_public_key() { RELEASE_PUBLIC_KEY_FILE="${test_root}/fixture-public.pem"; }
# Replace only network I/O. All production signature, parser, version, hash and
# promotion functions above execute unchanged against these local fixture files.
download_release_file() {
  source_file="${fixture_dir}/${1##*/}"
  [ -f "$source_file" ] || return 1
  [ "$(wc -c <"$source_file")" -le "${3:-536870912}" ] || return 1
  cp "$source_file" "$2"
}
sign_manifest() {
  openssl pkeyutl -sign -inkey "${test_root}/fixture-private.pem" -rawin \
    -in "${fixture_dir}/release-manifest.sha256" -out "${fixture_dir}/release-manifest.sha256.sig"
}
fixture() {
  printf 'v9.8.7+fixture\n' >"${fixture_dir}/agent-version.txt"
  printf 'v9.8.7+fixture\n' >"${fixture_dir}/cpanel-version.txt"
  for name in corade-linux-amd64 coradectl-linux-amd64 cpanel-linux-amd64 corade-install.sh; do
    printf 'non-executable fixture for %s\n' "$name" >"${fixture_dir}/${name}"
  done
  (cd "$fixture_dir" && sha256sum agent-version.txt cpanel-version.txt corade-linux-amd64 coradectl-linux-amd64 cpanel-linux-amd64 corade-install.sh) | sed 's/ \*/  /' >"${fixture_dir}/release-manifest.sha256"
  sign_manifest
}
reject() {
  label="$1"
  version=${2:-v9.8.7}
  target="${test_root}/rejected-$label"
  if load_signed_manifest https://fixture.invalid agent-version.txt "$version" && try_download corade-linux-amd64 "$target" https://fixture.invalid; then
    echo "$label unexpectedly accepted" >&2; exit 1
  fi
  test ! -e "$target"
}
fixture
load_signed_manifest https://fixture.invalid agent-version.txt v9.8.7
for name in corade-linux-amd64 coradectl-linux-amd64 cpanel-linux-amd64 corade-install.sh; do
  try_download "$name" "${test_root}/valid-$name" https://fixture.invalid
  cmp "${fixture_dir}/$name" "${test_root}/valid-$name"
done
load_signed_manifest https://fixture.invalid cpanel-version.txt v9.8.7+fixture
if command -v download_verified >/dev/null 2>&1; then
  REPOSITORY=fixture/fixture
  fail() { echo "$*" >&2; exit 1; }
  download_verified cpanel v9.8.7 cpanel-linux-amd64 "${test_root}/installed-panel"
  cmp "${fixture_dir}/cpanel-linux-amd64" "${test_root}/installed-panel"
fi
reject wrong-version v1.0.0
fixture; printf 'corrupt\n' >>"${fixture_dir}/corade-linux-amd64"; reject corrupt-artifact
fixture; printf 'tampered\n' >>"${fixture_dir}/release-manifest.sha256"; reject corrupt-manifest
fixture; rm "${fixture_dir}/release-manifest.sha256.sig"; reject missing-signature
fixture; dd if=/dev/zero of="${fixture_dir}/release-manifest.sha256.sig" bs=64 count=1 2>/dev/null; reject wrong-signature
fixture; printf 'v9.8.6\n' >"${fixture_dir}/agent-version.txt"; reject corrupt-version
fixture; cat "${fixture_dir}/release-manifest.sha256" >>"${test_root}/duplicate"; cat "${test_root}/duplicate" >>"${fixture_dir}/release-manifest.sha256"; sign_manifest; reject duplicate-entry
fixture; printf '%064d  ../outside\n' 0 >>"${fixture_dir}/release-manifest.sha256"; sign_manifest; reject path-traversal
echo 'Signed installer tests passed: valid assets, transport, trust anchor, version, tampering, missing/wrong signature, duplicate and traversal.'
