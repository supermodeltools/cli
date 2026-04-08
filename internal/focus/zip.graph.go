//go:build ignore

package ignore
// @generated supermodel-sidecar — do not edit
// [deps]
// imports     internal/api/client.go
// imports     internal/api/doc.go
// imports     internal/api/types.go
// imports     internal/config/config.go
// imports     internal/config/doc.go
// imported-by cmd/focus.go
// [calls]
// createZip → isGitRepo    internal/focus/zip.go:70
// createZip → gitArchive    internal/focus/zip.go:77
// createZip → walkZip    internal/focus/zip.go:85
// gitArchive ← createZip    internal/focus/zip.go:49
// isGitRepo ← createZip    internal/focus/zip.go:49
// walkZip ← createZip    internal/focus/zip.go:49
// [impact]
// risk        MEDIUM
// domains     CLIInfrastructure · SupermodelAPI
// direct      1
// transitive  2
// affects     cmd/focus.go
