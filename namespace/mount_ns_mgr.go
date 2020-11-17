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

	"github.com/YLonely/cer-manager/mount"
	"github.com/YLonely/cer-manager/rootfs"
	"golang.org/x/sys/unix"
)

func init() {
	PutNamespaceFunction(NamespaceOpCreate, MNT, populateRootfs)
	PutNamespaceFunction(NamespaceOpRelease, MNT, depopulateRootfs)
}

func NewMountManager(root string, capacity int, rootfsNames []string, provider rootfs.Provider) (Manager, error) {
	if capacity < 0 || len(rootfsNames) == 0 {
		return nil, errors.New("invalid init arguments for mnt namespace")
	}
	offset := 0
	var mounts []mount.Mount
	var overlays []mount.Overlay
	var err error
	rootfsParentDir := path.Join(root, "rootfs")
	if err = os.MkdirAll(rootfsParentDir, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create rootfs dir")
	}
	nsMgr := &mountManager{
		mgrs:       map[string]*subManager{},
		allBundles: map[int]string{},
		provider:   provider,
	}
	for _, name := range rootfsNames {
		mounts, err = provider.Prepare(name, name+"-rootfsKey")
		if err != nil {
			return nil, errors.Wrap(err, "error prepare rootfs for "+name)
		}
		if len(mounts) == 0 {
			return nil, errors.New("empty mount stack")
		}
		rootfsDir := path.Join(rootfsParentDir, name)
		if err = os.MkdirAll(rootfsDir, 0755); err != nil {
			return nil, errors.Wrap(err, "error create dir for "+name)
		}
		overlays, err = mount.ToOverlays(mounts)
		if err != nil && err != mount.OverlayTypeMismatchError {
			return nil, errors.Wrap(err, "failed to convert mounts")
		}
		if err == nil {
			makeOverlaysReadOnly(overlays)
			if err = mount.MountAllOverlays(overlays, rootfsDir); err != nil {
				return nil, errors.Wrap(err, "failed to mount overlay")
			}
		} else {
			if err = mount.MountAll(mounts, rootfsDir); err != nil {
				return nil, errors.Wrap(err, "failed to mount")
			}
		}
		nsMgr.mgrs[name] = &subManager{
			offset:      offset,
			usedBundles: map[int]string{},
		}
		if mgr, err := newGenericManager(capacity, MNT, nsMgr.makeCreateNewNamespace(name, rootfsDir)); err != nil {
			return nil, err
		} else {
			nsMgr.mgrs[name].mgr = mgr
		}
		offset++
	}
	return nsMgr, nil
}

var _ Manager = &mountManager{}

type mountManager struct {
	mgrs map[string]*subManager
	// allBundles maps namespace fd to it's bundle path
	allBundles map[int]string
	provider   rootfs.Provider
}

type subManager struct {
	mgr    *genericManager
	offset int
	// usedBundles maps namespace id to bundle dir
	usedBundles   map[int]string
	unusedBundles []string
	mutex         sync.Mutex
}

func (mgr *mountManager) Get(arg interface{}) (int, int, interface{}, error) {
	rootfsName := arg.(string)
	rootfsName = strings.TrimSuffix(rootfsName, "/")
	if sub, exists := mgr.mgrs[rootfsName]; exists {
		id, fd, _, err := sub.mgr.Get(nil)
		if err != nil {
			return -1, -1, nil, errors.Wrap(err, "rootfsName:"+rootfsName)
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
	return -1, -1, nil, errors.Errorf("Can't get namespace for root %s\n", rootfsName)
}

func (mgr *mountManager) Put(id int) error {
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

func (mgr *mountManager) Update(interface{}) error {
	return nil
}

func (mgr *mountManager) CleanUp() error {
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

var mounts = []struct {
	mount.Mount
	target string
}{
	{
		Mount: mount.Mount{
			Source:  "proc",
			Type:    "proc",
			Options: []string{},
		},
		target: "/proc",
	},
	{
		Mount: mount.Mount{
			Source: "udev",
			Type:   "devtmpfs",
			Options: []string{
				"nosuid",
				"strictatime",
				"mode=755",
				"size=65536k",
			},
		},
		target: "/dev",
	},
	{
		Mount: mount.Mount{
			Source: "devpts",
			Type:   "devpts",
			Options: []string{
				"nosuid",
				"noexec",
				"newinstance",
				"ptmxmode=0666",
				"mode=0620",
				"gid=5",
			},
		},
		target: "/dev/pts",
	},
	{
		Mount: mount.Mount{
			Source: "shm",
			Type:   "tmpfs",
			Options: []string{
				"nosuid",
				"noexec",
				"nodev",
				"mode=1777",
				"size=65536k",
			},
		},
		target: "/dev/shm",
	},
	{
		Mount: mount.Mount{
			Source: "mqueue",
			Type:   "mqueue",
			Options: []string{
				"nosuid",
				"noexec",
				"nodev",
			},
		},
		target: "/dev/mqueue",
	},
	{
		Mount: mount.Mount{
			Source: "sysfs",
			Type:   "sysfs",
			Options: []string{
				"nosuid",
				"noexec",
				"nodev",
				"ro",
			},
		},
		target: "/sys",
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

func (mgr *mountManager) makeCreateNewNamespace(rootfsName, rootfsPath string) func(NamespaceType) (int, error) {
	return func(t NamespaceType) (int, error) {
		bundle, err := createBundle()
		if err != nil {
			return -1, errors.Wrap(err, "Can't create bundle for "+rootfsName)
		}
		//call the namespace helper to create the ns
		helper, err := newNamespaceCreateHelper(t, rootfsPath, bundle)
		if err = helper.do(); err != nil {
			return -1, errors.Wrap(err, "Can't create new mnt namespace")
		}
		newNSFd := helper.getFd()
		sub := mgr.mgrs[rootfsName]
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
		if err := m.Mount.Mount(path.Join(rootfs, m.target)); err != nil {
			return errors.Wrapf(err, "mount(src:%s,dest:%s,type:%s) failed", m.Source, path.Join(rootfs, m.target), m.Type)
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
		paths = append(paths, mounts[i].target)
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

func makeOverlaysReadOnly(os []mount.Overlay) {
	n := len(os)
	last := os[n-1]
	upper, lowers := last.UpperDir, last.LowerDirs
	last.SetUpper("")
	last.SetWork("")
	last.SetLowers(append(lowers, upper))
}
