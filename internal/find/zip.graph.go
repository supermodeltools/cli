//go:build ignore

package ignore
// @generated supermodel-sidecar — do not edit
// [deps]
// imported-by cmd/find.go
// imported-by internal/find/integration_test.go
// [calls]
// createZip → isGitRepo    internal/find/zip.go:67
// createZip → gitArchive    internal/find/zip.go:74
// createZip → walkZip    internal/find/zip.go:82
// gitArchive ← createZip    internal/find/zip.go:46
// isGitRepo ← createZip    internal/find/zip.go:46
// walkZip ← createZip    internal/find/zip.go:46
// [impact]
// risk        MEDIUM
// domains     CLIInfrastructure · SupermodelAPI
// direct      2
// transitive  3
// affects     cmd/find.go · internal/find/integration_test.go
