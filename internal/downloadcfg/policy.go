package downloadcfg

// CollisionPolicy defines how to handle existing target files.
// Values: "error" | "overwrite" | "rename".
type CollisionPolicy string

const (
    CollisionError     CollisionPolicy = "error"
    CollisionOverwrite CollisionPolicy = "overwrite"
    CollisionRename    CollisionPolicy = "rename"
)

// StartOptions carries downloader-agnostic options for starting/resuming.
type StartOptions struct {
    Policy CollisionPolicy
}

// ParseCollisionPolicy converts a string to a CollisionPolicy with default.
func ParseCollisionPolicy(s string) CollisionPolicy {
    switch CollisionPolicy(s) {
    case CollisionOverwrite:
        return CollisionOverwrite
    case CollisionRename:
        return CollisionRename
    case CollisionError:
        fallthrough
    default:
        return CollisionError
    }
}

