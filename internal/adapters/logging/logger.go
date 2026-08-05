package logging

import (
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// New builds a stderr logger from format and level strings.
func New(format, level string) zerolog.Logger {
	var out io.Writer = os.Stderr
	if strings.ToLower(format) == "text" {
		out = zerolog.ConsoleWriter{Out: os.Stderr}
	}

	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	return zerolog.New(out).Level(lvl).With().Timestamp().Logger()
}
