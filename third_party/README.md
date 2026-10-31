# Audited Xray fork

`xray-core/` is the complete Go module from
`github.com/cedar2025/Xray-core@v0.0.0-20260409213332-f47935539965`.
Its LICENSE and original source files are retained. A local module replacement
preserves the custom bandwidth manager used by this Agent and makes the security
backport reproducible without modifying a global module cache at build time.

The only source changes from that base are the three files in upstream XTLS
commit `64fada32b5b9e6ae064a038fa0e4e2b766499bd5` (GHSA-5wf9-h793-w73c):

- TLS CA pinning rejects an empty verification name.
- gRPC and Hysteria pass their destination to TLS configuration.

The top-level module selects patched shared dependencies through Go MVS.
`go test ./internal/kernel/xray` must continue to verify both certificate-name
checks and the custom bandwidth manager whenever this fork is refreshed.

## Audited sing-box fork

`sing-box/` retains `github.com/cedar2025/sing-box@v1.14.0-alpha.2.0.20260316103356-2e665cb7e295`, including its LICENSE and custom features.
The v2rayhttp and v2raygrpclite clients now own their HTTP/2 connections and close
those connections through public APIs. This replaces the old unsafe pointer and
`go:linkname` access to a private x/net pool, removed by Go 1.27, and allows the
patched x/net dependency to be used without losing force-close behavior.

## Review and secret-scan policy

Both module trees are release inputs and must be committed with the Agent.
The Xray ignore file has two narrow exceptions to retain an upstream debug
source file and a buffer-test fixture, which its original patterns excluded.
Do not remove LICENSE files or replace these trees with build-time cache edits.

The root `.gitleaks.toml` permits only exact reviewed lines in exact paths:
upstream documentation/example passwords and WireGuard test vectors, the public
OAuth client ID in `service/ccm/credential.go`, and Go symbol references such as
`qtls.HKDFLabelKeyV2` and `blake2b.Size256` mistakenly classified as secrets.
The rest of the production vendor source remains scanned. Refreshing either
module requires reviewing changed exceptions, the Xray TLS regression and the
HTTP/2 reset/reconnect tests, as well as both Linux architecture builds.
