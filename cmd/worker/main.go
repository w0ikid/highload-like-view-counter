// Worker scheduling will be wired once flusher and reconciler are implemented.
package main

import (
	"log/slog"
	"os"
)

func main() {
	slog.Error("worker is not implemented yet: see internal/worker/README.md")
	os.Exit(1)
}
