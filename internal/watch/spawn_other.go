//go:build !unix

package watch

import "errors"

var errUnsupported = errors.New("background wipe timer is not supported on this platform")

func spawnDetached(string, []string) (int, error) { return 0, errUnsupported }
func terminate(int) error                         { return errUnsupported }
func isWatcher(int) bool                          { return false }
