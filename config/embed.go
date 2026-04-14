// Package config provides embedded seed data files.
package config

import "embed"

//go:embed models.seed.toml
var SeedFS embed.FS
