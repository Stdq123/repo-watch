# Diagnostic Remediation Implementation Plan

**Goal:** Resolve the reported build, CI and pointer-check diagnostics while preserving macOS 12 support and existing keychain entries.

**Architecture:** Keep the Go backend and native interfaces. Pin macOS builds to Go 1.26.10, migrate generic-password operations to SecItem against the existing file-based keychain, and keep narrowly scoped local patches of the Windows UI dependencies. Upgrade the GitHub Actions to Node 24 releases.

**Tech Stack:** Go, Windows APIs, macOS Security framework, GitHub Actions.

## Verification

- [x] Reproduce the Windows `go vet` warning and strict pointer-check failures before fixing them.
- [x] Verify Windows 64-bit and 32-bit test and vet commands; run native UI and tray tests with strict pointer checking.
- [x] Verify on macOS CI that old and new credentials round-trip in disposable keychains without modifying matching entries in other keychains.
- [x] Verify that both macOS executables declare a deployment target compatible with macOS 12 and compile without the reported deprecation and linker warnings.
- [x] Verify the Node 24 actions and artifact upload in GitHub Actions.
