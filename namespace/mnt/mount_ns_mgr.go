package mnt

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/mount"
	"github.com/YLonely/cer-manager/namespace"
	"github.com/YLonely/cer-manager/rootfs"
	"github.com/YLonely/criuimages"

	criutype "github.com/YLonely/criuimages/types"
	"golang.org/x/sys/unix"
)

func init() {
	namespace.PutNamespaceFunction(namespace.NamespaceFunctionKeyCreate, types.NamespaceMNT, populateBundle)
	namespace.PutNamespaceFunction(namespace.NamespaceFunctionKeyRelease, types.NamespaceMNT, depopulateBundle)
}

func NewManager(root string, capacities []int, refs []types.Reference, provider rootfs.Provider, supplier types.Supplier) (namespace.Manager, error) {
	var err error
	rootfsParentDir := path.Join(root, "rootfs")
	if err = os.MkdirAll(rootfsParentDir, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create rootfs dir")
	}
	m := &mountManager{
		root:        root,
		sets:        map[string]*namespace.Set{},
		allBundles:  map[int]string{},
		usedBundles: map[int]bundleInfo{},
		provider:    provider,
		supplier:    supplier,
	}
	for i, ref := range refs {
		m.initSet(ref, capacities[i])
	}
	return m, nil
}

var _ namespace.Manager = &mountManager{}

type mountManager struct {
	sets map[string]*namespace.Set
	// allBundles maps namespace fd to it's bundle path
	allBundles map[int]string
	provider   rootfs.Provider
	root       string
	// usedBundles maps fd to it's basic info
	usedBundles map[int]bundleInfo
	m           sync.Mutex
	supplier    types.Supplier
}

type bundleInfo struct {
	bundle string
	ref    types.Reference
	f      *os.File
}

func (m *mountManager) initSet(ref types.Reference, capacity int) error {
	mounts, err := m.provider.Prepare(ref, ref.Digest()+"-key")
	if err != nil {
		return errors.Wrap(err, "error prepare rootfs for "+ref.String())
	}
	if len(mounts) == 0 {
		return errors.New("empty mount stack")
	}
	rootfsDir := path.Join(m.root, "rootfs", ref.Digest())
	if err = os.MkdirAll(rootfsDir, 0755); err != nil {
		return errors.Wrap(err, "error create dir for "+ref.String())
	}
	if isOverlayMounts(mounts) {
		makeOverlaysReadOnly(mounts)
	}
	// umount it first, avoid stacked mount
	mount.UnmountAll(rootfsDir, 0)
	if err = mount.MountAll(mounts, rootfsDir); err != nil {
		return errors.Wrap(err, "failed to mount")
	}
	checkpoint, err := m.supplier.Get(ref)
	if err != nil {
		return errors.Wrapf(err, "failed to get checkpoint for %s", ref)
	}
	set, err := namespace.NewSet(capacity, m.makeNewNamespaceCreator(rootfsDir, checkpoint), m.makePreRelease())
	if err != nil {
		return errors.Wrapf(err, "failed to create namespace set for %s", ref)
	}
	m.sets[ref.Digest()] = set
	return nil
}

func (mgr *mountManager) Get(ref types.Reference, extraRefs ...types.Reference) (fd int, info interface{}, err error) {
	if len(extraRefs) > 0 {
		err = errors.New("multiple references is not supported")
		return
	}
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if set, exists := mgr.sets[ref.Digest()]; exists {
		f := set.Get()
		if f == nil {
			err = errors.Errorf("MNT namespace of %s is used up", ref)
			return
		}
		info = mgr.allBundles[int(f.Fd())]
		fd = int(f.Fd())
		mgr.usedBundles[fd] = bundleInfo{
			ref:    ref,
			bundle: info.(string),
			f:      f,
		}
		return
	}
	err = errors.Errorf("MNT namespace of %s is not managed by us", ref)
	return
}

func (mgr *mountManager) Put(fd int) error {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	info, exists := mgr.usedBundles[fd]
	if !exists {
		return errors.Errorf("invalid fd %d", fd)
	}
	go func() {
		mgr.m.Lock()
		defer mgr.m.Unlock()
		if _, exists := mgr.usedBundles[fd]; !exists {
			return
		}

		defer info.f.Close()
		defer delete(mgr.usedBundles, fd)
		set, exists := mgr.sets[info.ref.Digest()]
		if !exists {
			panic(errors.Errorf("MNT namespace set of %s does not exist", info.ref))
		}
		if err := mgr.makePreRelease()(info.f); err != nil {
			log.Raw().WithError(err).Errorf("failed to release the MNT namespace of fd %d", info.f.Fd())
		}
		if err := set.CreateOne(); err != nil {
			log.Raw().WithError(err).Errorf("failed to create a new MNT namespace for %s", info.ref)
		}
	}()
	return nil
}

func (mgr *mountManager) Update(ref types.Reference, capacity int) error {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	set, exists := mgr.sets[ref.Digest()]
	if !exists {
		if err := mgr.initSet(ref, capacity); err != nil {
			return err
		}
		return nil
	}
	return set.Update(capacity)
}

func (m *mountManager) makePreRelease() func(*os.File) error {
	return func(f *os.File) error {
		bundle, exists := m.allBundles[int(f.Fd())]
		if !exists {
			return errors.Errorf("bundle path of fd %d does not exist", f.Fd())
		}
		helper, err := namespace.NewNamespaceExecEnterHelper(
			namespace.NamespaceFunctionKeyRelease,
			types.NamespaceMNT,
			fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), int(f.Fd())),
			map[string]string{
				"bundle": bundle,
			},
		)
		if err != nil {
			return errors.Wrapf(err, "failed to create namespace helper for %d with bundle %s", f.Fd(), bundle)
		}
		if err = helper.Do(true); err != nil {
			return errors.Wrapf(err, "failed to release bundle %s of fd %d", bundle, f.Fd())
		}
		return nil
	}
}

func (mgr *mountManager) CleanUp() error {
	var last error
	for _, info := range mgr.usedBundles {
		log.Raw().Warnf("bundle %s of %s is being used", info.bundle, info.ref)
		if err := mgr.makePreRelease()(info.f); err != nil {
			last = err
			log.Raw().WithError(err).Errorf("failed to release bundle %s of %s", info.bundle, info.ref)
		}
	}
	rootfsParentDir := path.Join(mgr.root, "rootfs")
	for digest, set := range mgr.sets {
		// umount the rootfs with digest
		rootfsDir := path.Join(rootfsParentDir, digest)
		if err := mount.UnmountAll(rootfsDir, 0); err != nil {
			last = err
			log.Raw().WithError(err).Errorf("failed to unmount rootfs %s", rootfsDir)
		}
		if err := mgr.provider.Remove(digest + "-key"); err != nil {
			last = err
			log.Raw().WithError(err).Errorf("failed to remove rootfs %s", digest)
		}
		if err := set.CleanUp(); err != nil {
			last = err
			log.Raw().WithError(err).Errorf("failed to clean up the namespace set of %s", digest)
		}
	}
	return last
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
	{
		Mount: mount.Mount{
			Source: "tmpfs",
			Type:   "tmpfs",
			Options: []string{
				"size=65536k",
				"mode=755",
			},
		},
		target: "/run",
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

func (mgr *mountManager) makeNewNamespaceCreator(rootfsPath, checkpointPath string) func() (*os.File, error) {
	return func() (*os.File, error) {
		bundle, err := createBundle()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create bundle")
		}
		//call the namespace helper to create the ns
		helper, err := namespace.NewNamespaceExecCreateHelper(
			namespace.NamespaceFunctionKeyCreate,
			types.NamespaceMNT,
			map[string]string{
				"src":        rootfsPath,
				"bundle":     bundle,
				"checkpoint": checkpointPath,
			},
		)
		if err = helper.Do(false); err != nil {
			return nil, errors.Wrap(err, "failed to execute the namespace helper")
		}
		defer helper.Release()
		newNSFile, err := namespace.OpenNSFile(types.NamespaceMNT, helper.Cmd.Process.Pid)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open namespace file")
		}
		mgr.allBundles[int(newNSFile.Fd())] = bundle
		return newNSFile, nil
	}
}

func populateRootfs(rootfs, checkpoint string) error {
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
		if err := unix.Mount(joinedPath, joinedPath, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_REC, ""); err != nil {
			return errors.Wrapf(err, "failed to make %s readonly", joinedPath)
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
	if err := restoreExtraMountpoints(rootfs, checkpoint); err != nil {
		return errors.Wrap(err, "failed to restore extra mount points")
	}

	//restore files in fs
	if err := restoreFiles(rootfs, checkpoint); err != nil {
		return errors.Wrap(err, "failed to restore files")
	}
	return nil
}

func restoreExtraMountpoints(rootfs, checkpoint string) error {
	const (
		mountpointsPrefix = "mountpoints-"
	)
	mp := map[string]struct{}{
		"/": {},
	}
	for _, m := range mounts {
		mp[m.target] = struct{}{}
	}
	for _, m := range append(append([]string{}, readonlyPaths...), maskedPaths...) {
		mp[m] = struct{}{}
	}
	mpFilePath := ""
	infos, err := ioutil.ReadDir(checkpoint)
	if err != nil {
		return err
	}
	for _, info := range infos {
		if strings.HasPrefix(info.Name(), mountpointsPrefix) {
			mpFilePath = path.Join(checkpoint, info.Name())
			break
		}
	}
	if mpFilePath == "" {
		return errors.New("failed to find mountpoints.img")
	}
	img, err := criuimages.New(mpFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open image %s", mpFilePath)
	}
	defer img.Close()
	entry := &criutype.MntEntry{}
	for {
		err := img.ReadOne(entry)
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "failed to read entry")
		}
		if _, alreadyMounted := mp[entry.GetMountpoint()]; alreadyMounted {
			continue
		}
		// we only handle the basic bind mount here for simplicity
		flags := unix.MS_BIND
		if entry.GetExtKey() == "" {
			return errors.Errorf("encounter a non-binded mount at %s", entry.GetMountpoint())
		}
		if (entry.GetFlags() & unix.MS_RDONLY) > 0 {
			flags |= unix.MS_RDONLY
		}
		if err := unix.Mount(entry.GetExtKey(), path.Join(rootfs, entry.GetMountpoint()), "", uintptr(flags), ""); err != nil {
			return errors.Wrapf(err, "failed to bind mount %s to %s", entry.GetExtKey(), entry.GetMountpoint())
		}
	}
	return nil
}

func restoreFiles(rootfs, checkpoint string) error {
	const (
		tarGzPrefix       = "tmpfs-dev-"
		tarGzSuffix       = ".tar.gz.img"
		mountpointsPrefix = "mountpoints-"
	)
	items, err := ioutil.ReadDir(checkpoint)
	if err != nil {
		return err
	}
	mountpointsFilePath := ""
	for _, i := range items {
		if !i.IsDir() && strings.HasPrefix(i.Name(), mountpointsPrefix) {
			mountpointsFilePath = path.Join(checkpoint, i.Name())
			break
		}
	}
	if mountpointsFilePath == "" {
		return errors.New("failed to find mountpoints-%d.img")
	}
	img, err := criuimages.New(mountpointsFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to create mountpoint image")
	}
	defer img.Close()
	entries, err := readAllCandidates(rootfs, img)
	if err != nil {
		return err
	}
	for _, i := range items {
		if !i.IsDir() && strings.HasPrefix(i.Name(), tarGzPrefix) {
			devIDStr := strings.TrimPrefix(strings.TrimSuffix(i.Name(), tarGzSuffix), tarGzPrefix)
			devID, err := strconv.Atoi(devIDStr)
			if err != nil {
				return errors.Wrap(err, "can't convert devID string")
			}
			if err := doRestore(rootfs, entries, uint32(devID), path.Join(checkpoint, i.Name())); err != nil {
				return errors.Wrapf(err, "failed to restore with dev id %d", devID)
			}
		}
	}
	return nil
}

func doRestore(rootfs string, entries map[uint32][]*criutype.MntEntry, devID uint32, restoreFilePath string) error {
	var list []*criutype.MntEntry
	var exists bool
	if list, exists = entries[devID]; !exists {
		return nil
	}
	if len(list) == 0 {
		return errors.New("list empty")
	}
	sort.Slice(list, func(i, j int) bool {
		mpA, mpB := list[i].GetMountpoint(), list[j].GetMountpoint()
		dirsA, dirsB := strings.Split(mpA, "/"), strings.Split(mpB, "/")
		return len(dirsA) < len(dirsB)
	})
	mountpoint := list[0].GetMountpoint()
	target := path.Join(rootfs, mountpoint)
	cmd := exec.Command("/bin/tar", "--extract", "--gzip", "--no-unquote", "--no-wildcards", "--directory="+target, "-f", restoreFilePath)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to untar file %s to %s", restoreFilePath, target)
	}
	return nil
}

func readAllCandidates(rootfs string, img *criuimages.Image) (map[uint32][]*criutype.MntEntry, error) {
	entries := map[uint32][]*criutype.MntEntry{}
	readonlys := map[string]struct{}{
		"/sys": {},
	}
	for _, p := range readonlyPaths {
		readonlys[p] = struct{}{}
	}
	for _, p := range maskedPaths {
		readonlys[p] = struct{}{}
	}
	for {
		entry := criutype.MntEntry{}
		err := img.ReadOne(&entry)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to read image")
		}
		if entry.GetExtKey() != "" {
			continue
		}
		if _, exists := readonlys[path.Clean(entry.GetMountpoint())]; exists {
			continue
		}
		mountpoint := path.Join(rootfs, entry.GetMountpoint())
		stat, err := os.Stat(mountpoint)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get file info")
		}
		if !stat.IsDir() {
			continue
		}
		devID := entry.GetRootDev()
		if _, exists := entries[devID]; !exists {
			entries[devID] = []*criutype.MntEntry{}
		}
		entries[devID] = append(entries[devID], &entry)
	}
	return entries, nil
}

func populateBundle(args map[string]interface{}) ([]byte, error) {
	src, ok := args["src"].(string)
	if !ok || src == "" {
		return nil, errors.New("src must be provided")
	}
	bundle, ok := args["bundle"].(string)
	if !ok || bundle == "" {
		return nil, errors.New("bundle must be provided")
	}
	checkpoint, ok := args["checkpoint"].(string)
	if !ok || checkpoint == "" {
		return nil, errors.New("checkpoint must be provided")
	}
	//isolate the root
	if err := unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return nil, err
	}
	return nil, doPopulate(src, bundle, checkpoint)
}

func doPopulate(src, bundle, checkpoint string) error {
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
	return populateRootfs(rootfs, checkpoint)
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

func depopulateBundle(args map[string]interface{}) ([]byte, error) {
	bundle, ok := args["bundle"].(string)
	if !ok || bundle == "" {
		return nil, errors.New("bundle must be provided")
	}
	rootfs := path.Join(bundle, "rootfs")
	if err := depopulateRootfs(rootfs); err != nil {
		return nil, err
	}
	// umount rootfs
	if err := unix.Unmount(rootfs, unix.MNT_DETACH); err != nil {
		return nil, errors.Wrap(err, "failed to unmount rootfs")
	}
	// remove bundle
	if err := os.RemoveAll(bundle); err != nil {
		return nil, errors.Wrap(err, "failed to remove bundle")
	}
	return nil, nil
}

func reMakeDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0711); err != nil {
		return err
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
