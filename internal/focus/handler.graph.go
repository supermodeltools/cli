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
// imported-by cmd/focus.go
// [calls]
// Run ← init    cmd/focus.go:10
// Run → getGraph    internal/focus/handler.go:342
// Run → extract    internal/focus/handler.go:72
// Run → render    internal/focus/handler.go:286
// buildCalleesOf ← extract    internal/focus/handler.go:72
// estimateTokens ← extract    internal/focus/handler.go:72
// extract ← Run    internal/focus/handler.go:55
// extract → pathMatches    internal/focus/handler.go:278
// extract → reachableImports    internal/focus/handler.go:173
// extract → functionNodesForFile    internal/focus/handler.go:206
// extract → buildCalleesOf    internal/focus/handler.go:225
// extract → extractTypes    internal/focus/handler.go:235
// extract → estimateTokens    internal/focus/handler.go:264
// extractTypes ← extract    internal/focus/handler.go:72
// functionNodesForFile ← extract    internal/focus/handler.go:72
// getGraph ← Run    internal/focus/handler.go:55
// getGraph → HashFile    internal/cache/cache.go:59
// getGraph → Get    internal/cache/cache.go:28
// getGraph → Start    internal/ui/output.go:74
// getGraph → Put    internal/cache/cache.go:44
// pathMatches ← extract    internal/focus/handler.go:72
// printMarkdown ← render    internal/focus/handler.go:286
// reachableImports ← extract    internal/focus/handler.go:72
// render ← Run    internal/focus/handler.go:55
// render → printMarkdown    internal/focus/handler.go:293
// render → JSON    internal/ui/output.go:47
// [impact]
// risk        MEDIUM
// domains     CLIInfrastructure · SupermodelAPI
// direct      1
// transitive  2
// affects     cmd/focus.go
