// Package themes contains the theme files shipped with myde.
package themes

import "embed"

// Builtin contains every built-in JSON theme.
//
//go:embed *.json
var Builtin embed.FS
