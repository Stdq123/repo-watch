# Local Windows dependency patches

`win` is copied from `github.com/lxn/win` at
`v0.0.0-20210218163916-a377121e959e`, and `walk` contains the root and
`declarative` packages from `github.com/lxn/walk` at
`v0.0.0-20210112085537-c389da54e794`. Their original licenses are included in
each directory. The module replacements in `go.mod` use these local copies.

The local patches preserve Win32 integer-resource semantics under Go's
`checkptr`, keep enumeration callback pointers alive, and look up tray icons
without storing a Go pointer in Windows window userdata. When updating either
dependency, reapply or replace these changes and run the Windows UI tests with
`-gcflags=all=-d=checkptr=2`.
