package wire

// Level represents the severity of a log event.
type Level uint8

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// ParseLevel converts a string to a Level. Returns LevelInfo and false if
// the string is not recognised.
func ParseLevel(s string) (Level, bool) {
	switch s {
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn", "warning":
		return LevelWarn, true
	case "error":
		return LevelError, true
	case "fatal":
		return LevelFatal, true
	default:
		return LevelInfo, false
	}
}
