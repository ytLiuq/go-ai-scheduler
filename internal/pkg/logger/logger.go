package logger

import (
	"log"
	"os"
)

// New returns a standard logger with a fixed service prefix.
func New(serviceName string) *log.Logger {
	return log.New(os.Stdout, "["+serviceName+"] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)
}

