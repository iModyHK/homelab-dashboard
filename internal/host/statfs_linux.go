//go:build linux

package host

import "syscall"

func statMount(path string) (MountUsage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return MountUsage{}, err
	}
	bsize := uint64(st.Bsize)
	total := st.Blocks * bsize
	free := st.Bavail * bsize
	used := (st.Blocks - st.Bfree) * bsize
	return MountUsage{Total: total, Free: free, Used: used}, nil
}
