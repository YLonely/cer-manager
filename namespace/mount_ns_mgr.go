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

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/mount"
	"github.com/YLonely/cer-manager/rootfs"
	"github.com/YLonely/cer-manager/services"
	"golang.org/x/sys/unix"
)

func init() {
	PutNamespaceFunction(NamespaceOpCreate, types.NamespaceMNT, populateBundle)
	PutNamespaceFunction(NamespaceOpRelease, types.NamespaceMNT, depopulateBundle)
	PutNamespaceFunction(NamespaceOpReset, types.NamespaceMNT, resetBundle)
}

func NewMountManager(root string, capacity int, rootfsNames []string, provider rootfs.Provider) (Manager, error) {
	if capacity < 0 || len(rootfsNames) == 0 {
		return nil, errors.New("invalid init arguments for mnt namespace")
	}
	offset := 0
	var mounts []mount.Mount
	var err error
	rootfsParentDir := path.Join(root, "rootfs")
	if err = os.MkdirAll(rootfsParentDir, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create rootfs dir")
	}
	nsMgr := &mountManager{
		root:        root,
		mgrs:        map[string]*subManager{},
		allBundles:  map[int]string{},
		usedBundles: map[int]item{},
		provider:    provider,
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
		if isOverlayMounts(mounts) {
			makeOverlaysReadOnly(mounts)
		}
		if err = mount.MountAll(mounts, rootfsDir); err != nil {
			return nil, errors.Wrap(err, "failed to mount")
		}
		nsMgr.mgrs[name] = &subManager{
			offset: offset,
		}
		if mgr, err := newGenericManager(capacity, types.NamespaceMNT, nsMgr.makeCreateNewNamespace(name, rootfsDir)); err != nil {
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
	root       string
	// usedBundles maps id to it's bundle path and fd
	usedBundles map[int]item
	m           sync.Mutex
}

type item struct {
	fd     int
	bundle string
}

type subManager struct {
	mgr    *genericManager
	offset int
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
		retBundle := mgr.allBundles[fd]
		mgr.m.Lock()
		defer mgr.m.Unlock()
		mgr.usedBundles[id] = item{
			fd:     fd,
			bundle: retBundle,
		}
		return retID, fd, retBundle, nil
	}
	return -1, -1, nil, errors.Errorf("Can't get namespace for root %s\n", rootfsName)
}

func (mgr *mountManager) Put(id int) error {
	i, exists := mgr.usedBundles[id]
	if !exists {
		return errors.New("invalid id")
	}
	h, err := newNamespaceResetHelper(types.NamespaceMNT, os.Getpid(), i.fd, i.bundle)
	if err != nil {
		log.Logger(services.NamespaceService, "Put").WithError(err).Error("failed to create helper")
		return nil
	}
	if err = h.do(); err != nil {
		log.Logger(services.NamespaceService, "Put").WithError(err).Error("error reset namespace")
		return nil
	}
	mgr.m.Lock()
	delete(mgr.usedBundles, id)
	mgr.m.Unlock()
	offset := id % len(mgr.mgrs)
	for _, sub := range mgr.mgrs {
		if sub.offset == offset {
			innerID := id / len(mgr.mgrs)
			if err := sub.mgr.Put(innerID); err != nil {
				return err
			}
			return nil
		}
	}
	return errors.New("invalid id")
}

func (mgr *mountManager) Update(interface{}) error {
	return nil
}

func (mgr *mountManager) CleanUp() error {
	var failed []string
	for fd, bundle := range mgr.allBundles {
		helper, err := newNamespaceReleaseHelper(types.NamespaceMNT, os.Getpid(), fd, bundle)
		if err != nil {
			failed = append(failed, fmt.Sprintf("Failed to create ns helper for fd %d and bundle %s with error %s", fd, bundle, err.Error()))
			continue
		}
		if err = helper.do(); err != nil {
			failed = append(failed, fmt.Sprintf("Failed to execute helper for fd %d and bundle %s with error %s", fd, bundle, err.Error()))
		}
	}
	rootfsParentDir := path.Join(mgr.root, "rootfs")
	for name, sub := range mgr.mgrs {
		// umount the rootfs with name
		rootfsDir := path.Join(rootfsParentDir, name)
		if err := mount.UnmountAll(rootfsDir, 0); err != nil {
			failed = append(failed, fmt.Sprintf("umount rootfs %s with error %s", rootfsDir, err.Error()))
		}
		if err := mgr.provider.Remove(name + "-rootfsKey"); err != nil {
			failed = append(failed, fmt.Sprintf("remove rootfs %s with error %s", name, err.Error()))
		}
		if err := sub.mgr.CleanUp(); err != nil {
			failed = append(failed, fmt.Sprintf("cleanup sub-manager %s with error %s", name, err.Error()))
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

func (mgr *mountManager) makeCreateNewNamespace(rootfsName, rootfsPath string) func(types.NamespaceType) (*os.File, error) {
	return func(t types.NamespaceType) (*os.File, error) {
		bundle, err := createBundle()
		if err != nil {
			return nil, errors.Wrap(err, "Can't create bundle for "+rootfsName)
		}
		//call the namespace helper to create the ns
		helper, err := newNamespaceCreateHelper(t, rootfsPath, bundle)
		if err = helper.do(); err != nil {
			return nil, errors.Wrap(err, "Can't create new mnt namespace")
		}
		newNSFile := helper.nsFile()
		mgr.allBundles[int(newNSFile.Fd())] = bundle
		return newNSFile, nil
	}
}

func populateRootfs(rootfs string) error {
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

func populateBundle(args ...interface{}) error {
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
	m := mount.Mount{
		Source: "overlay",
		Type:   "overlay",
	}
	m.SetWork(path.Join(bundle, "work"))
	m.SetUpper(path.Join(bundle, "upper"))
	m.SetLowers([]string{src})
	if err := m.Mount(rootfs); err != nil {
		return errors.Wrapf(err, "mount rootfs %s with overlay failed", rootfs)
	}
	if err := unix.Chmod(rootfs, 0755); err != nil {
		return errors.Wrap(err, "can not chmod")
	}
	return populateRootfs(rootfs)
}

func depopulateRootfs(rootfs string) error {
	var failed []string
	paths := append(maskedPaths, readonlyPaths...)
	for i := len(mounts) - 1; i >= 0; i-- {
		paths = append(paths, mounts[i].target)
	}
	// umount all the mount point in rootfs
	for _, p := range paths {
		joinedPath := path.Join(rootfs, p)
		if err := unix.Unmount(joinedPath, unix.MNT_DETACH); err != nil && !os.IsNotExist(err) {
			failed = append(failed, "unmount "+joinedPath+" with error "+err.Error())
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return errors.New(strings.Join(failed, ";"))
}

func depopulateBundle(args ...interface{}) error {
	l := len(args)
	if l < 1 {
		return errors.New("No enough args")
	}
	bundle, ok := args[0].(string)
	if !ok {
		return errors.New("Can't convert args[0] to string")
	}
	rootfs := path.Join(bundle, "rootfs")
	if err := depopulateRootfs(rootfs); err != nil {
		return err
	}
	// umount rootfs
	if err := unix.Unmount(rootfs, unix.MNT_DETACH); err != nil {
		return errors.Wrap(err, "failed to unmount rootfs")
	}
	// remove bundle
	if err := os.RemoveAll(bundle); err != nil {
		return errors.Wrap(err, "failed to remove bundle")
	}
	return nil
}

func resetBundle(args ...interface{}) error {
	l := len(args)
	if l < 1 {
		return errors.New("no enough args")
	}
	bundle, ok := args[0].(string)
	if !ok {
		return errors.New("can't convert args[0] to string")
	}
	rootfs := path.Join(bundle, "rootfs")
	upper := path.Join(bundle, "upper")
	work := path.Join(bundle, "work")
	// clean up work and upper
	if err := removeContents(upper); err != nil {
		return errors.Wrap(err, "failed to clean upper")
	}
	if err := removeContents(work); err != nil {
		return errors.Wrap(err, "failed to clean work")
	}
	// unmount and remount rootfs
	if err := depopulateRootfs(rootfs); err != nil {
		return errors.Wrap(err, "failed to depopulate rootfs")
	}
	if err := populateRootfs(rootfs); err != nil {
		return errors.Wrap(err, "failed to populate rootfs")
	}
	return nil
}

func removeContents(p string) error {
	dirs, err := ioutil.ReadDir(p)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if err := os.RemoveAll(path.Join(p, d.Name())); err != nil {
			return err
		}
	}
	return nil
}

func makeOverlaysReadOnly(ms []mount.Mount) {
	n := len(ms)
	last := &ms[n-1]
	upper, lowers := last.Upper(), last.Lowers()
	last.SetUpper("")
	last.SetWork("")
	last.SetLowers(append([]string{upper}, lowers...))
}

func isOverlayMounts(ms []mount.Mount) bool {
	n := len(ms)
	return ms[n-1].IsOverlay()
}
