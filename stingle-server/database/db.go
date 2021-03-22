// Package database implements all the storage requirement of the stingle server
// using a local filesystem. It doesn't use any external database server.
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"stingle-server/log"
)

var (
	// Set this only for tests.
	CurrentTimeForTesting int64 = 0
)

// New returns an initialized database that uses dir for storage.
func New(dir string) *Database {
	for _, d := range []string{"users", "blobs", "albums"} {
		sub := filepath.Join(dir, d)
		if err := os.MkdirAll(sub, 0700); err != nil {
			log.Panicf("os.MkdirAll(%q): %v", sub, err)
		}

	}
	return &Database{dir: dir}
}

// Implements all the storage requirements of the stingle server using a local
// filesystem.
type Database struct {
	dir string
}

// Dir returns the directory where the database stores its data.
func (d Database) Dir() string {
	return d.dir
}

// nowInMS returns the current time in ms.
func nowInMS() int64 {
	if CurrentTimeForTesting != 0 {
		return CurrentTimeForTesting
	}
	return time.Now().UnixNano() / 1000000 // ms
}

// boolToString converts a bool to "0" or "1".
func boolToString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func showCallStack() {
	pc := make([]uintptr, 10)
	n := runtime.Callers(2, pc)
	if n == 0 {
		return
	}
	frames := runtime.CallersFrames(pc[:n])

	log.Debug("Call Stack")
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "stingle-server") {
			break
		}
		fl := fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
		log.Debugf("   %-15s %s", fl, frame.Function)
		if !more {
			break
		}
	}
}
