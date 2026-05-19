//go:build windows

package config

import (
	"os"
	"sync"
)

// Windows fallback: process-local mutex (no cross-process flock on Windows without cgo).
var globalMu sync.Mutex

func lockExclusive(_ *os.File) error {
	globalMu.Lock()
	return nil
}

func unlockFile(_ *os.File) {
	globalMu.Unlock()
}
