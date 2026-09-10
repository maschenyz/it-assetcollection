package logger

import (
	"fmt"
	"os"
	"time"
)

func now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func Info(tag, message string) {
	fmt.Printf("[%s] [%s] %s\n", now(), tag, message)
}

func Error(tag, message string) {
	fmt.Fprintf(os.Stderr, "[%s] [%s] ERROR: %s\n", now(), tag, message)
}
