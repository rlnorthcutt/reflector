// Package presets embeds Reflector's example route packs. The same files
// are loaded directly by --preset and extracted to disk by `reflector
// init`; this directory is also the repository's browsable examples/
// directory, so all three views stay in sync by construction.
package presets

import "embed"

//go:embed rest-api flaky slow auth big-payloads
var FS embed.FS

// Names lists the available preset names, in a stable display order.
var Names = []string{"rest-api", "flaky", "slow", "auth", "big-payloads"}
