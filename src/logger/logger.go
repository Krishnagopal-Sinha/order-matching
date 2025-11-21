package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var Logger zerolog.Logger
var logFile *os.File

func InitLogger() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	logFilePath := os.Getenv("LOG_FILE")

	if logFilePath == "" || logFilePath == "none" || logFilePath == "disabled" {
		logFile = nil
	} else {
		var err error
		logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Error().Err(err).Msg("Failed to open log file, using stdout only")
			logFile = nil
		}
	}

	logFormat := os.Getenv("LOG_FORMAT")

	var writers []io.Writer

	if logFormat == "pretty" {
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		writers = append(writers, consoleWriter)
	} else {
		writers = append(writers, os.Stdout)
	}

	if logFile != nil {
		writers = append(writers, logFile)
	}

	multiWriter := io.MultiWriter(writers...)

	Logger = zerolog.New(multiWriter).With().
		Timestamp().
		Logger()

	log.Logger = Logger

	if logFile != nil {
		Logger.Info().
			Str("log_file", logFilePath).
			Str("log_level", level.String()).
			Msg("Logger initialized - writing to console and file")
	} else {
		Logger.Info().
			Str("log_level", level.String()).
			Msg("Logger initialized - writing to console only")
	}
}

func CloseLogger() {
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
		logFile = nil
	}
}

func GetLogger() zerolog.Logger {
	return Logger
}

