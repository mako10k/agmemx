package cli

import "io"

// tryCompletion handles `agmemx __completion --bash [words...]`.
// handled is false until the completion protocol is implemented.
func tryCompletion(args []string, stdout io.Writer) (bool, int) {
	_, _ = args, stdout
	return false, 0
}
