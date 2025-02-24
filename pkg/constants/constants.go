package constants

// Despite the package name these are vars, so that they can be updated at link time.
var (
	ToolName      = "backup-tool"
	ToolShortName = "bt"
	Version       = "0.0.1-dev"
	ImageRegistry = "ghcr.io/soliddowant"
	ImageName     = ImageRegistry + "/" + ToolName
	ImageTag      = Version
	FullImageName = ImageName + ":" + ImageTag
)
