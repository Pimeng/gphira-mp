package config

import (
	"os"
	"time"
)

// Watcher polls a config file for changes and invokes the callback.
type Watcher struct {
	path     string
	interval time.Duration
	onChange func()
	stop     chan struct{}
}

// NewWatcher creates a new config file watcher.
func NewWatcher(path string, interval time.Duration, onChange func()) *Watcher {
	return &Watcher{
		path:     path,
		interval: interval,
		onChange: onChange,
		stop:     make(chan struct{}),
	}
}

// Start begins watching the config file.
func (w *Watcher) Start() {
	go w.run()
}

func (w *Watcher) run() {
	var lastMod time.Time
	if info, err := os.Stat(w.path); err == nil {
		lastMod = info.ModTime()
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				w.onChange()
			}
		case <-w.stop:
			return
		}
	}
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	close(w.stop)
}
