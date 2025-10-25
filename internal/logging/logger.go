package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Default logger instance
	defaultLogger *zap.Logger
)

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
		defaultLogger, _ = zap.NewProduction()
	}
	return defaultLogger
}

// Sync flushes any buffered log entries
func Sync() {
	if defaultLogger != nil {
		defaultLogger.Sync()
	}
}
