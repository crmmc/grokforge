package modelconfig

import "embed"

//go:embed models.toml
var EmbeddedFS embed.FS
