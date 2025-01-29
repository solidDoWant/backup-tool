package constants

const (
	ToolName      = "backup-tool"
	Version       = "0.0.0-dev"
	ImageRegistry = "ghcr.io/soliddowant"
	ImageName     = ImageRegistry + "/" + ToolName
	ImageTag      = Version
	FullImageName = ImageName + ":" + ImageTag
)
