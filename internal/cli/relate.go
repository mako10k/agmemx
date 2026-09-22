package cli

import (
	"encoding/json"
	"io"

	"agmemx/internal/xdg"
)

func handleRelate(stdout, _ io.Writer, _ xdg.Roots, _ options, _ string, _ map[string]json.RawMessage) int {
	writeReject(stdout, reject("invalid_command", 2))
	return 2
}
