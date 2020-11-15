package namespace

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"golang.org/x/sys/unix"
)

func init() {
	PutNamespaceFunction(NamespaceOpCreate, MNT, populateRootfs)
	PutNamespaceFunction(NamespaceOpRelease, MNT, depopulateRootfs)
}

func newMountNamespaceManager(capacity int, roots []string) (namespaceManager, error) {
	if capacity < 0 || len(roots) == 0 {
		return nil, errors.New("invalid init arguments for mnt namespace")
	}
	nsMgr := &mountNamespaceManager{
		mgrs:       map[string]*subManager{},
		allBundles: map[int]string{},
	}
	offset := 0
	for _, root := range roots {
		root = strings.TrimSuffix(root, "/")
		nsMgr.mgrs[root] = &subManager{
			offset:      offset,
			usedBundles: map[int]string{},
		}
		if mgr, err := newGenericNamespaceManager(capacity, MNT, nsMgr.makeCreateNewNamespace(root)); err != nil {
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
	mgrs map[string]*subManager
	// allBundles maps namespace fd to it's bundle path
	allBundles map[int]string
}

type subManager struct {
	mgr    *genericNamespaceManager
	offset int
	// usedBundles maps namespace id to bundle dir
	usedBundles   map[int]string
	unusedBundles []string
	mutex         sync.Mutex
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
		l := len(sub.unusedBundles)
		if l == 0 {
			panic("The number of the namespace and rootfs didn't match")
		}
		retRoot := sub.unusedBundles[l-1]
		sub.unusedBundles = sub.unusedBundles[:l-1]
		sub.usedBundles[id] = retRoot
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
			if root, exists := sub.usedBundles[innerID]; !exists {
				panic("Rootfs %d isn't in use in mnt ns manager")
			} else {
				delete(sub.usedBundles, innerID)
				sub.unusedBundles = append(sub.unusedBundles, root)
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
	var failed []string
	for fd, bundle := range mgr.allBundles {
		helper, err := newNamespaceReleaseHelper(MNT, os.Getpid(), fd, bundle)
		if err != nil {
			failed = append(failed, fmt.Sprintf("Failed to create ns helper for fd %d and bundle %s with error %s", fd, bundle, err.Error()))
			continue
		}
		if err = helper.do(); err != nil {
			failed = append(failed, fmt.Sprintf("Failed to execute helper for fd %d and bundle %s with error %s", fd, bundle, err.Error()))
		}
	}
	for _, sub := range mgr.mgrs {
		if err := sub.mgr.CleanUp(); err != nil {
			failed = append(failed, "Failed to cleanup sub manager")
		}
	}
	if len(failed) != 0 {
		return errors.New(strings.Join(failed, ";"))
	}
	return nil
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

func createBundle() (string, error) {
	// create the bundle dir
	bundle, err := ioutil.TempDir("", ".cer.bundle.*")
	if err != nil {
		return "", err
	}
	// create the upper, work and rootfs dir for the overlay mount
	upperPath := filepath.Join(bundle, "upper")
	if err = os.Mkdir(upperPath, 0711); err != nil {
		return "", err
	}
	workPath := filepath.Join(bundle, "work")
	if err = os.Mkdir(workPath, 0711); err != nil {
		return "", err
	}
	rootfsPath := filepath.Join(bundle, "rootfs")
	if err = os.Mkdir(rootfsPath, 0711); err != nil {
		return "", err
	}
	return bundle, nil
}

func (mgr *mountNamespaceManager) makeCreateNewNamespace(root string) func(NamespaceType) (int, error) {
	return func(t NamespaceType) (int, error) {
		bundle, err := createBundle()
		if err != nil {
			return -1, errors.Wrap(err, "Can't create bundle for "+root)
		}
		//call the namespace helper to create the ns
		helper, err := newNamespaceCreateHelper(t, root, bundle)
		if err = helper.do(); err != nil {
			return -1, errors.Wrap(err, "Can't create new mnt namespace")
		}
		newNSFd := helper.getFd()
		sub := mgr.mgrs[root]
		sub.unusedBundles = append(sub.unusedBundles, bundle)
		mgr.allBundles[newNSFd] = bundle
		return newNSFd, nil
	}
}

func populateRootfs(args ...interface{}) error {
	l := len(args)
	if l < 2 {
		return errors.New("No enough arguments")
	}
	src, ok := args[0].(string)
	if !ok {
		return errors.New("Can't convert arg[0] to string")
	}
	bundle, ok := args[1].(string)
	if !ok {
		return errors.New("Can't convert arg[1] to string")
	}
	//isolate the root
	if err := unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return err
	}
	// mount the src dir to rootfs dir in bundle
	rootfs := path.Join(bundle, "rootfs")
	mountData := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", src, path.Join(bundle, "upper"), path.Join(bundle, "work"))
	if err := unix.Mount("overlay", rootfs, "overlay", uintptr(0), mountData); err != nil {
		return errors.Wrapf(err, "mount(overlay,%s,overlay,0,%s) failed", rootfs, mountData)
	}
	//mount general fs
	for _, m := range mounts {
		flags, data := parseMountOptions(m.options)
		if err := unix.Mount(m.src, path.Join(rootfs, m.dest), m.mtype, uintptr(flags), data); err != nil {
			return errors.Wrapf(err, "mount(%s,%s,%s,%d,%s) failed", m.src, path.Join(rootfs, m.dest), m.mtype, flags, data)
		}
	}
	//make readonly paths
	for _, p := range readonlyPaths {
		joinedPath := path.Join(rootfs, p)
		if err := unix.Mount(joinedPath, joinedPath, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
			if !os.IsNotExist(err) {
				return errors.Wrapf(err, "Make %s readonly failed", joinedPath)
			}
		}
	}
	//make masked paths
	for _, p := range maskedPaths {
		joinedPath := path.Join(rootfs, p)
		if err := unix.Mount("/dev/null", joinedPath, "", unix.MS_BIND, ""); err != nil && !os.IsNotExist(err) {
			if err == unix.ENOTDIR {
				if err = unix.Mount("tmpfs", joinedPath, "tmpfs", unix.MS_RDONLY, ""); err != nil {
					return errors.Wrapf(err, "Make %s masked failed", joinedPath)
				}
			}
		}
	}
	return nil
}

func depopulateRootfs(args ...interface{}) error {
	var failed []string
	l := len(args)
	if l < 1 {
		return errors.New("No enough args")
	}
	bundle, ok := args[0].(string)
	if !ok {
		return errors.New("Can't convert args[0] to string")
	}
	rootfs := path.Join(bundle, "rootfs")
	paths := append(maskedPaths, readonlyPaths...)
	for i := len(mounts) - 1; i >= 0; i-- {
		paths = append(paths, mounts[i].dest)
	}
	// umount all the mount point in rootfs
	for _, p := range paths {
		joinedPath := path.Join(rootfs, p)
		if err := unix.Unmount(joinedPath, unix.MNT_DETACH); err != nil && !os.IsNotExist(err) {
			failed = append(failed, err.Error()+":failed to unmount "+joinedPath)
		}
	}
	// umount rootfs
	if err := unix.Unmount(rootfs, unix.MNT_DETACH); err != nil {
		failed = append(failed, err.Error()+":failed to unmount "+rootfs)
	}
	// remove bundle
	if err := os.RemoveAll(bundle); err != nil {
		failed = append(failed, err.Error()+":failed to remove "+bundle)
	}
	if len(failed) != 0 {
		return errors.New(strings.Join(failed, ";"))
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
