package token

import "log/slog"

func safeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered", "name", name, "panic", r)
			}
		}()
		fn()
	}()
}
