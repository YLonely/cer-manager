package ccfs

import (
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/YLonely/cer-manager/checkpoint"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// NewProvider returns a provider based on ccfs
func NewProvider(registry string) (checkpoint.Provider, error) {
	if err := mountCCFS(); err != nil {
		return nil, errors.Wrap(err, "failed to mount ccfs")
	}
	return &provider{
		registry: registry,
	}, nil
}

const (
	mountpoint = "/tmp/.ccfs"
	mountsFile = "/proc/mounts"
)

var _ checkpoint.Provider = &provider{}

type provider struct {
	registry string
}

func (p *provider) Prepare(ref string, target string) error {
	checkpointDir := path.Join(mountpoint, ref)
	if _, err := os.Stat(checkpointDir); err == nil {
		return nil
	}
	// we make a dir named ref and the ccfs will do the rest of the work
	if err := os.Mkdir(checkpointDir, 0555); err != nil {
		return errors.Wrap(err, "failed to create dir "+checkpointDir)
	}
	if err := unix.Mount(checkpointDir, target, "", unix.MS_BIND, ""); err != nil {
		return errors.Wrap(err, "failed to bind mount to target path")
	}
	return nil
}

func (p *provider) Remove(target string) error {
	unix.Unmount(target, unix.MNT_DETACH)
	return nil
}

func mountCCFS() error {
	mounted, err := checkMount()
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}
	//TODO: mount ccfs here
	return nil
}

func checkMount() (bool, error) {
	content, err := ioutil.ReadFile(mountsFile)
	if err != nil {
		return false, errors.Wrap(err, "failed to read /proc/mounts")
	}
	mounts := strings.Split(string(content), "\n")
	for _, m := range mounts {
		parts := strings.Split(m, " ")
		if len(parts) < 2 {
			return false, errors.New("error parse mountpoints")
		}
		fsName, mp := parts[0], parts[1]
		if fsName == "ccfs" && path.Clean(mp) == path.Clean(mountpoint) {
			return true, nil
		}
	}
	return false, nil
}
