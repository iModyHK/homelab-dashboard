//go:build !linux

package host

import "errors"

func statMount(path string) (MountUsage, error) {
	return MountUsage{}, errors.New("mount statistics are only available on linux")
}
