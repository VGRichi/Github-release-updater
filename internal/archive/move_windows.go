package archive

import (
	"errors"

	"golang.org/x/sys/windows"
)

// otherDrive reports whether a rename failed only because the target is on another drive.
func otherDrive(err error) bool { return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) }
