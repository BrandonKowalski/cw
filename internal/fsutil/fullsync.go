package fsutil

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

var fullFsyncCmd int

func FullSync(f *os.File) error {
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", f.Name(), err)
	}
	if fullFsyncCmd == 0 {
		return nil
	}
	if _, err := unix.FcntlInt(f.Fd(), fullFsyncCmd, 0); err != nil && !unsupported(err) {
		return fmt.Errorf("full fsync %s: %w", f.Name(), err)
	}
	return nil
}

func unsupported(err error) bool {
	return errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTTY)
}
