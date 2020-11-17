package rootfs

import "github.com/YLonely/cer-manager/mount"

// Provider provides rootfs to other services
type Provider interface {
	// Prepare prepares the rootfs of name and returns a stack of mount
	Prepare(name string, key string) ([]mount.Mount, error)
	// Remove removes the resources bonded with the key
	Remove(key string) error
}
