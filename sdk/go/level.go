package opentrace

import "github.com/opentrace/opentrace-go/internal/wire"

// Level represents the severity of a log event.
type Level = wire.Level

const (
	LevelDebug = wire.LevelDebug
	LevelInfo  = wire.LevelInfo
	LevelWarn  = wire.LevelWarn
	LevelError = wire.LevelError
	LevelFatal = wire.LevelFatal
)

// ParseLevel converts a string to a Level. Returns LevelInfo and false if
// the string is not recognised.
func ParseLevel(s string) (Level, bool) { return wire.ParseLevel(s) }
