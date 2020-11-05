package namespace

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"golang.org/x/sys/unix"
)

func newMountNamespaceManager(capacity int, roots []string) (namespaceManager, error) {
	if capacity < 0 || len(roots) == 0 {
		return nil, errors.New("invalid init arguments for mnt namespace")
	}
	nsMgr := &mountNamespaceManager{
		mgrs:     map[string]*subManager{},
		rootsFds: map[int]string{},
	}
	offset := 0
	for _, root := range roots {
		root = strings.TrimSuffix(root, "/")
		nsMgr.mgrs[root] = &subManager{
			offset:    offset,
			usedRoots: map[int]string{},
		}
		if mgr, err := newGenericNamespaceManager(capacity, MNT, nsMgr.makeMountHook(root)); err != nil {
			return nil, err
		} else {
			nsMgr.mgrs[root].mgr = mgr
		}
		offset++
	}
	return nsMgr, nil
}

var _ namespaceManager = &mountNamespaceManager{}

type mountNamespaceManager struct {
	mgrs     map[string]*subManager
	rootsFds map[int]string
}

type subManager struct {
	mgr          *genericNamespaceManager
	offset       int
	usedRoots    map[int]string
	mountedRoots []string
	mutex        sync.Mutex
}

func (mgr *mountNamespaceManager) Get(arg interface{}) (int, int, interface{}, error) {
	root := arg.(string)
	root = strings.TrimSuffix(root, "/")
	if sub, exists := mgr.mgrs[root]; exists {
		id, fd, _, err := sub.mgr.Get(nil)
		if err != nil {
			return -1, -1, nil, errors.Wrap(err, "root "+root)
		}
		retID := id*len(mgr.mgrs) + sub.offset
		sub.mutex.Lock()
		defer sub.mutex.Unlock()
		l := len(sub.mountedRoots)
		if l == 0 {
			panic("The number of the namespace and rootfs didn't match")
		}
		retRoot := sub.mountedRoots[l-1]
		sub.mountedRoots = sub.mountedRoots[:l-1]
		sub.usedRoots[id] = retRoot
		return retID, fd, retRoot, nil
	}
	return -1, -1, nil, errors.Errorf("Can't get namespace for root %s\n", root)
}

func (mgr *mountNamespaceManager) Put(id int) error {
	offset := id % len(mgr.mgrs)
	for _, sub := range mgr.mgrs {
		if sub.offset == offset {
			innerID := id / len(mgr.mgrs)
			if err := sub.mgr.Put(innerID); err != nil {
				return err
			}
			sub.mutex.Lock()
			defer sub.mutex.Unlock()
			if root, exists := sub.usedRoots[innerID]; !exists {
				panic("Rootfs %d isn't in use in mnt ns manager")
			} else {
				delete(sub.usedRoots, innerID)
				sub.mountedRoots = append(sub.mountedRoots, root)
			}
			return nil
		}
	}
	return nil
}

func (mgr *mountNamespaceManager) Update(interface{}) error {
	return nil
}

func (mgr *mountNamespaceManager) CleanUp() error {
	var err error
	for fd, root := range mgr.rootsFds {
		if err = depopulateRootfs(fd, root); err != nil {
			return err
		}
	}
	for _, sub := range mgr.mgrs {
		if errClean := sub.mgr.CleanUp(); errClean != nil {
			if err != nil {
				err = errors.Wrap(err, errClean.Error())
			} else {
				err = errClean
			}
		}
	}
	return err
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
		mtype: "devtmpfs",
		src:   "udev",
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

func depopulateRootfs(fd int, root string) error {
	var err error
	scriptPath := "/var/lib/crdaemon/scripts/depopulate_rootfs.sh"
	if _, err = os.Stat(scriptPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err = createScript(scriptPath); err != nil {
			return err
		}
	}
	cmd := exec.Command(scriptPath, strconv.Itoa(os.Getpid()), strconv.Itoa(fd), root)
	if err = cmd.Run(); err != nil {
		return err
	}
	return os.Remove(root)
}

func createScript(scriptPath string) error {
	if err := os.MkdirAll(path.Dir(scriptPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(scriptPath)
	if err != nil {
		return err
	}
	f.WriteString("#!/bin/sh\npid=$1\nns_fd=$2\nroot=$3\nnsenter --mount=/proc/$pid/fd/$ns_fd umount -Rl $root")
	f.Close()
	os.Chmod(scriptPath, 0755)
	return nil
}

func (mgr *mountNamespaceManager) makeMountHook(root string) func(int, int) error {
	return func(oldNS, newNS int) error {
		return mgr.prepareRootfs(root, newNS)
	}
}

func (mgr *mountNamespaceManager) prepareRootfs(root string, newNS int) error {
	//isolate the root
	if err := unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return err
	}
	//create a temp dir as the bundle
	tempDir, err := ioutil.TempDir("", ".crdaemon.rootfs.*")
	if err != nil {
		return err
	}
	// create the upper, work and rootfs dir for the overlay mount
	upperPath := filepath.Join(tempDir, "upper")
	if err = os.Mkdir(upperPath, 0711); err != nil {
		return err
	}
	workPath := filepath.Join(tempDir, "work")
	if err = os.Mkdir(workPath, 0711); err != nil {
		return err
	}
	rootfsPath := filepath.Join(tempDir, "rootfs")
	if err = os.Mkdir(rootfsPath, 0711); err != nil {
		return err
	}
	// mount the root dir to rootfs
	if err = unix.Mount(root, tempDir, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return err
	}
	if err = unix.Mount("", tempDir, "", unix.MS_PRIVATE|unix.MS_REC, ""); err != nil {
		return err
	}
	//mount general fs
	for _, m := range mounts {
		flags, data := parseMountOptions(m.options)
		if err := unix.Mount(m.src, path.Join(tempDir, m.dest), m.mtype, uintptr(flags), data); err != nil {
			return err
		}
	}
	//make readonly paths
	for _, p := range readonlyPaths {
		joinedPath := path.Join(tempDir, p)
		if err := unix.Mount(joinedPath, joinedPath, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		}
	}
	//make masked paths
	for _, p := range maskedPaths {
		joinedPath := path.Join(tempDir, p)
		if err := unix.Mount("/dev/null", joinedPath, "", unix.MS_BIND, ""); err != nil && !os.IsNotExist(err) {
			if err == unix.ENOTDIR {
				if err = unix.Mount("tmpfs", joinedPath, "tmpfs", unix.MS_RDONLY, ""); err != nil {
					return err
				}
			}
		}
	}
	sub := mgr.mgrs[root]
	sub.mountedRoots = append(sub.mountedRoots, tempDir)
	mgr.rootsFds[newNS] = tempDir
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
