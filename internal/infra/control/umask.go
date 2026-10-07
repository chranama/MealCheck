package control

import "golang.org/x/sys/unix"

func sysUmask() int      { return unix.Umask(0077) }
func restoreUmask(v int) { unix.Umask(v) }
