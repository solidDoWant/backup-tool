package constants

import "strings"

// Despite the package name these are vars, so that they can be updated at link time.
var (
	ToolName      = "backup-tool"
	Version       = "v0.0.1-dev"
	ImageRegistry = "ghcr.io/soliddowant"
	ImageName     = ImageRegistry + "/" + ToolName
	ImageTag      = strings.TrimPrefix(Version, "v")
	FullImageName = ImageName + ":" + ImageTag
)
