// Package config manages the user's Supermodel configuration stored at
// ~/.supermodel/config.yaml. It handles loading, saving, and validating
// config values such as the API key, default output format, and API base URL.
//
// This is a shared kernel package. It must contain no business logic.
// Slice packages under internal/ may import it freely.
package config
