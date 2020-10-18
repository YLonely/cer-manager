package namespace

import (
	"os"
	"path"
	"strings"

	"golang.org/x/sys/unix"
)

func newMountNamespaceManager(capacity int, roots []string) (*namespaceManager, error) {
}

type mountNamespaceManager struct {
	mgrs map[string]*genericNamespaceManager
}

type mount struct {
	dest    string
	mtype   string
	src     string
	options []string
}

var mounts = []mount{
	{
		dest:    "/proc",
		mtype:   "proc",
		src:     "proc",
		options: []string{},
	},
	{
		dest:  "/dev",
		mtype: "tmpfs",
		src:   "tmpfs",
		options: []string{
			"nosuid",
			"strictatime",
			"mode=755",
			"size=65536k",
		},
	},
	{
		dest:  "/dev/pts",
		mtype: "devpts",
		src:   "devpts",
		options: []string{
			"nosuid",
			"noexec",
			"newinstance",
			"ptmxmode=0666",
			"mode=0620",
			"gid=5",
		},
	},
	{
		dest:  "/dev/shm",
		mtype: "tmpfs",
		src:   "shm",
		options: []string{
			"nosuid",
			"noexec",
			"nodev",
			"mode=1777",
			"size=65536k",
		},
	},
	{
		dest:  "/dev/mqueue",
		mtype: "mqueue",
		src:   "mqueue",
		options: []string{
			"nosuid",
			"noexec",
			"nodev",
		},
	},
	{
		dest:  "/sys",
		mtype: "sysfs",
		src:   "sysfs",
		options: []string{
			"nosuid",
			"noexec",
			"nodev",
			"ro",
		},
	},
}

var readonlyPaths = []string{
	"/proc/bus",
	"/proc/fs",
	"/proc/irq",
	"/proc/sys",
	"/proc/sysrq-trigger",
}

var maskedPaths = []string{
	"/proc/acpi",
	"/proc/asound",
	"/proc/kcore",
	"/proc/keys",
	"/proc/latency_stats",
	"/proc/timer_list",
	"/proc/timer_stats",
	"/proc/sched_debug",
	"/sys/firmware",
	"/proc/scsi",
}

func makeMountHook(root string) func() error {
	return func() error {
		return prepareRootfs(root)
	}
}

func prepareRootfs(root string) error {
	//isolate the root
	if err := unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return err
	}
	//mount general fs
	for _, m := range mounts {
		flags, data := parseMountOptions(m.options)
		if err := unix.Mount(m.src, path.Join(root, m.dest), m.mtype, uintptr(flags), data); err != nil {
			return err
		}
	}
	//make readonly paths
	for _, p := range readonlyPaths {
		joinedPath := path.Join(root, p)
		if err := unix.Mount(joinedPath, joinedPath, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		}
	}
	//make masked paths
	for _, p := range maskedPaths {
		joinedPath := path.Join(root, p)
		if err := unix.Mount("/dev/null", joinedPath, "", unix.MS_BIND, ""); err != nil && !os.IsNotExist(err) {
			if err == unix.ENOTDIR {
				if err = unix.Mount("tmpfs", joinedPath, "tmpfs", unix.MS_RDONLY, ""); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func parseMountOptions(options []string) (int, string) {
	flags := map[string]int{
		"async":       unix.MS_ASYNC,
		"noatime":     unix.MS_NOATIME,
		"nodev":       unix.MS_NODEV,
		"nodiratime":  unix.MS_NODIRATIME,
		"dirsync":     unix.MS_DIRSYNC,
		"noexec":      unix.MS_NOEXEC,
		"relatime":    unix.MS_RELATIME,
		"strictatime": unix.MS_STRICTATIME,
		"nosuid":      unix.MS_NOSUID,
		"ro":          unix.MS_RDONLY,
		"sync":        unix.MS_SYNC,
	}
	flag := 0
	datas := []string{}
	for _, option := range options {
		if v, exists := flags[option]; exists {
			flag |= v
		} else {
			datas = append(datas, option)
		}
	}
	data := strings.Join(datas, ",")
	return flag, data
}
