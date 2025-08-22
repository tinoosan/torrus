package v1

import "errors"

var (
	ErrDownloadCtx   = errors.New("download missing in context")
	ErrDesiredStatus = errors.New("desired status missing in context")
	ErrDesiredStatusJSON = errors.New("desired status is required")
	ErrTargetPath = errors.New("targetPath is required")
	ErrContentType = errors.New("Content-Type must be application/json")
	ErrMagnetURI = errors.New("invalid magnet link")

)

