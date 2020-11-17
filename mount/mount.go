package mount

import (
	mnt "github.com/containerd/containerd/mount"
)

// Mount is a 'Mount' struct from containerd
type Mount mnt.Mount

func (m *Mount) Mount(target string) error {
	mm := mnt.Mount(*m)
	return mm.Mount(target)
}

func MountAll(mounts []Mount, target string) error {
	for _, m := range mounts {
		if err := m.Mount(target); err != nil {
			return err
		}
	}
	return nil
}

func UnmountAll(target string, flags int) error {
	return mnt.UnmountAll(target, flags)
}

func Unmount(target string, flags int) error {
	return mnt.Unmount(target, flags)
}
