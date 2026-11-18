package fsutil

import "golang.org/x/sys/unix"

func init() {
	fullFsyncCmd = unix.F_FULLFSYNC
}
