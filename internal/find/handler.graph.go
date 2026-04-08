//go:build ignore

package ignore

// @generated supermodel-sidecar — do not edit
// [deps]
// imports     internal/api/client.go
// imports     internal/api/doc.go
// imports     internal/api/types.go
// imports     internal/cache/cache.go
// imports     internal/cache/doc.go
// imports     internal/cache/fingerprint.go
// imports     internal/config/config.go
// imports     internal/config/doc.go
// imports     internal/ui/doc.go
// imports     internal/ui/output.go
// imported-by cmd/find.go
// imported-by internal/find/integration_test.go
// [calls]
// Run ← init    cmd/find.go:10
// Run ← TestIntegration_Run_Find    internal/find/integration_test.go:16
// Run ← TestIntegration_Run_Find_JSON    internal/find/integration_test.go:31
// Run ← TestIntegration_Run_Find_NoMatch    internal/find/integration_test.go:47
// Run ← TestIntegration_Run_Find_KindFilter    internal/find/integration_test.go:62
// Run → getGraph    internal/find/handler.go:144
// Run → search    internal/find/handler.go:49
// Run → printMatches    internal/find/handler.go:120
// Run → ParseFormat    internal/ui/output.go:24
// getGraph ← Run    internal/find/handler.go:36
// getGraph → Analyze    internal/api/client.go:41
// getGraph → HashFile    internal/cache/cache.go:59
// getGraph → Get    internal/cache/cache.go:28
// getGraph → Start    internal/ui/output.go:74
// getGraph → Put    internal/cache/cache.go:44
// printMatches ← Run    internal/find/handler.go:36
// printMatches → JSON    internal/ui/output.go:47
// search ← Run    internal/find/handler.go:36
// [impact]
// risk        MEDIUM
// domains     CLIInfrastructure · SupermodelAPI
// direct      2
// transitive  3
// affects     cmd/find.go · internal/find/integration_test.go
