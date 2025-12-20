package logging

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// MaxLogFieldLength is the maximum length for log field values.
	// Longer values will be truncated with "..." suffix.
	MaxLogFieldLength = 500
)

var (
	// Default logger instance
	defaultLogger *zap.Logger
)

// Truncate truncates a string to MaxLogFieldLength characters.
// If the string is longer, it appends "..." to indicate truncation.
func Truncate(s string) string {
	if len(s) <= MaxLogFieldLength {
		return s
	}
	return s[:MaxLogFieldLength] + "..."
}

// TruncateN truncates a string to n characters.
// If the string is longer, it appends "..." to indicate truncation.
func TruncateN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TruncateSlice truncates a string slice for logging.
// If the slice has more than maxItems elements, it truncates and adds count info.
func TruncateSlice(items []string, maxItems int) []string {
	if len(items) <= maxItems {
		return items
	}
	result := make([]string, maxItems+1)
	copy(result, items[:maxItems])
	result[maxItems] = "... and " + itoa(len(items)-maxItems) + " more"
	return result
}

// itoa is a simple int to string conversion without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b strings.Builder
	if i < 0 {
		b.WriteByte('-')
		i = -i
	}
	var digits []byte
	for i > 0 {
		digits = append(digits, byte('0'+i%10))
		i /= 10
	}
	for j := len(digits) - 1; j >= 0; j-- {
		b.WriteByte(digits[j])
	}
	return b.String()
}

// InitLogger initializes the default logger
func InitLogger() error {
	config := zap.NewProductionConfig()

	// Set log level based on environment
	if os.Getenv("LOG_LEVEL") == "debug" {
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	} else {
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Configure output
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	// Configure encoder
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.StacktraceKey = "stacktrace"

	// Create logger
	var err error
	defaultLogger, err = config.Build()
	if err != nil {
		return err
	}

	// Replace global logger
	zap.ReplaceGlobals(defaultLogger)
	return nil
}

// Logger returns the default logger instance
func Logger() *zap.Logger {
	if defaultLogger == nil {
		// Fallback to basic logger if not initialized
		logger, err := zap.NewProduction()
		if err != nil {
			// If production logger fails, try development logger as last resort
			logger, err = zap.NewDevelopment()
			if err != nil {
				// If all else fails, use Nop logger to prevent nil pointer
				logger = zap.NewNop()
			}
		}
		defaultLogger = logger
	}
	return defaultLogger
}

// Sync flushes any buffered log entries
func Sync() error {
	if defaultLogger != nil {
		if err := defaultLogger.Sync(); err != nil {
			// Sync errors are often safe to ignore (e.g., /dev/stderr on Linux)
			// but we log them for debugging
			defaultLogger.Error("failed to sync logger", zap.Error(err))
			return err
		}
	}
	return nil
}
