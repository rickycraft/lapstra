package core

import (
	"io"
	"log/syslog"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
)

var (
	once       sync.Once
	logger     *log.Logger
	apstraUser string
)

func SetApstraUser(username string) {
	apstraUser = username
}

func Logger() *log.Logger {
	once.Do(initLogger)
	return logger
}

func WithPrefix(prefix string) *log.Logger {
	return Logger().WithPrefix(prefix)
}

func initLogger() {
	level := log.InfoLevel
	if os.Getenv("LAPSTRA_DEBUG") != "" {
		level = log.DebugLevel
	}

	output := io.Writer(os.Stdout)
	if syslogWriter, err := syslog.New(syslog.LOG_INFO|syslog.LOG_LOCAL1, "lapstra"); err == nil {
		output = io.MultiWriter(output, customSyslogWriter{writer: syslogWriter})
	}

	logger = log.NewWithOptions(output, log.Options{
		ReportTimestamp: true,
		Level:           level,
	})
}

type customSyslogWriter struct {
	writer *syslog.Writer
}

func (w customSyslogWriter) Write(p []byte) (int, error) {
	message := strings.TrimRight(string(p), "\n")[20:]
	user := apstraUser
	message = user + " " + message

	return len(p), w.writer.Info(message)
}
