package flow

import (
	"log/slog"
)

// SafeGo runs fn in a goroutine and recovers panics to keep the process alive.
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered", "name", name, "panic", r)
			}
		}()
		fn()
	}()
}
