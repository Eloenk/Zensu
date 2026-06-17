package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	logFile *os.File
	logger  *log.Logger
)

func Init() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	zensuDir := filepath.Join(dir, "zensu")
	if err := os.MkdirAll(zensuDir, 0755); err != nil {
		return err
	}
	logPath := filepath.Join(zensuDir, "henzuku.log")

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	logFile = file

	mw := io.MultiWriter(os.Stdout, logFile)
	logger = log.New(mw, "", 0)
	
	Infof("STARTUP", "Logger initialized at %s", sanitizePath(logPath))
	return nil
}

func Close() {
	if logFile != nil {
		Infof("SHUTDOWN", "Logger shutting down")
		logFile.Close()
	}
}

func sanitizePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		p = strings.ReplaceAll(p, home, "<Home>")
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		p = strings.ReplaceAll(p, wd, "<AppDir>")
	}
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
