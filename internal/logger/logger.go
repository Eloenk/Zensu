package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	logFile *os.File
	logger  *log.Logger
)

type stdoutWriter struct {
	w io.Writer
}

func (sw *stdoutWriter) Write(p []byte) (n int, err error) {
	_, _ = sw.w.Write(p)
	return len(p), nil
}

func Init() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	zensuDir := filepath.Join(dir, "zensu")
	if err := os.MkdirAll(zensuDir, 0755); err != nil {
		return err
	}
	baseLogPath := filepath.Join(zensuDir, "henzuku.log")
	var file *os.File
	var openErr error
	finalLogPath := baseLogPath
	for i := 0; i < 10; i++ {
		path := baseLogPath
		if i > 0 {
			path = filepath.Join(zensuDir, fmt.Sprintf("henzuku_%d.log", i))
		}
		file, openErr = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if openErr == nil {
			finalLogPath = path
			break
		}
	}
	if openErr != nil {
		return openErr
	}
	logFile = file

	mw := io.MultiWriter(logFile, &stdoutWriter{w: os.Stdout})
	logger = log.New(mw, "", 0)
	
	Infof("STARTUP", "Logger initialized at %s", sanitizePath(finalLogPath))
	return nil
}

func Close() {
	if logFile != nil {
		Infof("SHUTDOWN", "Logger shutting down")
		logFile.Close()
	}
}

type WailsLogger struct{}

func (l *WailsLogger) Print(message string)   { logMessage("PRINT", "WAILS", message) }
func (l *WailsLogger) Trace(message string)   { logMessage("TRACE", "WAILS", message) }
func (l *WailsLogger) Debug(message string)   { logMessage("DEBUG", "WAILS", message) }
func (l *WailsLogger) Info(message string)    { logMessage("INFO", "WAILS", message) }
func (l *WailsLogger) Warning(message string) { logMessage("WARN", "WAILS", message) }
func (l *WailsLogger) Error(message string)   { logMessage("ERROR", "WAILS", message) }
func (l *WailsLogger) Fatal(message string)   { logMessage("FATAL", "WAILS", message) }

func sanitizePath(p string) string {
	return p
}

func logMessage(level, code, msg string) {
	if logger == nil {
		fmt.Printf("[%s] %s\n", level, msg)
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	var formatted string
	if code != "" {
		formatted = fmt.Sprintf("%s [%s] [%s] %s", timestamp, level, code, msg)
	} else {
		formatted = fmt.Sprintf("%s [%s] %s", timestamp, level, msg)
	}
	logger.Println(formatted)
}

func Info(code, msg string) {
	logMessage("INFO", code, sanitizePath(msg))
}

func Infof(code, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	Info(code, msg)
}

func Error(code, msg string) {
	logMessage("ERROR", code, sanitizePath(msg))
}

func Errorf(code, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	Error(code, msg)
}

func Warn(code, msg string) {
	logMessage("WARN", code, sanitizePath(msg))
}

func Warnf(code, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	Warn(code, msg)
}
