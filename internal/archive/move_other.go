//go:build !windows

package archive

import (
	"errors"
	"syscall"
)

// otherDrive reports whether a rename failed only because the target is on another drive.
func otherDrive(err error) bool { return errors.Is(err, syscall.EXDEV) }
