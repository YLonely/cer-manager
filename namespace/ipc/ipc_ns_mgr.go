package ipc

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/namespace"
	"github.com/YLonely/cer-manager/namespace/generic"
	"github.com/YLonely/cer-manager/services"
	"github.com/YLonely/cer-manager/utils"
	"github.com/YLonely/criuimages"
	criutype "github.com/YLonely/criuimages/types"
	"github.com/YLonely/ipcgo"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func init() {
	namespace.PutNamespaceFunction(functionKeyCollect, types.NamespaceIPC, collect)
	namespace.PutNamespaceFunction(namespace.NamespaceFunctionKeyCreate, types.NamespaceIPC, populateNamespace)
}

func NewManager(root string, capacity int, imageRefs []string, supplier services.CheckpointSupplier) (namespace.Manager, error) {
	if capacity < 0 || len(imageRefs) == 0 {
		return nil, errors.New("invalid initial args for ipc manager")
	}
	defaultVars, err := getDefaultNamespace()
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect varaibles from new ipc namespace")
	}
	normals, specials, err := devide(imageRefs, defaultVars, supplier)
	if err != nil {
		return nil, err
	}
	ret := &manager{
		supplier: supplier,
		managers: map[string]*generic.GenericManager{},
		usedID:   map[int]*generic.GenericManager{},
	}
	if len(normals) != 0 {
		mgr, err := generic.NewManager(capacity*len(normals), types.NamespaceIPC, nil)
		if err != nil {
			return nil, err
		}
		ret.managers[ipcTypeNormal] = mgr
	}
	if len(specials) != 0 {
		for _, ref := range specials {
			cp, err := supplier.Get(ref)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get checkpoint path for "+ref)
			}
			mgr, err := generic.NewManager(capacity, types.NamespaceIPC, makeCreateNewIPCNamespace(cp))
			if err != nil {
				return nil, err
			}
			ret.managers[ref] = mgr
		}
	}
	return ret, nil
}

const (
	functionKeyCollect namespace.NamespaceFunctionKey = "collect"
	ipcTypeNormal                                     = "normal"
	pageSize                                          = 1 << 12
)

type manager struct {
	managers map[string]*generic.GenericManager
	supplier services.CheckpointSupplier
	mu       sync.Mutex
	// usedID maps id to the manager that generates it
	usedID map[int]*generic.GenericManager
}

func (m *manager) Get(arg interface{}) (fd int, info interface{}, err error) {
	ref, ok := arg.(string)
	if !ok {
		err = errors.New("arg must be a string")
		return
	}
	mgr, exists := m.managers[ref]
	if !exists {
		err = errors.Errorf("ipc namespace of %s does not exist", ref)
	}
	fd, info, err = mgr.Get(nil)
	if err != nil {
		err = errors.Wrap(err, "failed to get namespace for "+ref)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usedID[fd] = mgr
	return
}

func (m *manager) Put(fd int) error {
	mgr, exists := m.usedID[fd]
	if !exists {
		return errors.New("invalid id")
	}
	if err := mgr.Put(fd); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.usedID, fd)
	return nil
}

func (m *manager) Update(interface{}) error {
	return nil
}

func (m *manager) CleanUp() error {
	var failed []string
	for ref, mgr := range m.managers {
		if err := mgr.CleanUp(); err != nil {
			failed = append(failed, fmt.Sprintf("clean up manager for %s, error %s", ref, err.Error()))
		}
	}
	if len(failed) != 0 {
		return errors.New(strings.Join(failed, ";"))
	}
	return nil
}

func makeCreateNewIPCNamespace(checkpointPath string) func(t types.NamespaceType) (*os.File, error) {
	return func(types.NamespaceType) (*os.File, error) {
		h, err := namespace.NewNamespaceExecCreateHelper(
			namespace.NamespaceFunctionKeyCreate,
			types.NamespaceIPC,
			map[string]string{
				"checkpoint": checkpointPath,
			},
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ipc create helper")
		}
		if err = h.Do(false); err != nil {
			return nil, err
		}
		f, err := namespace.OpenNSFile(types.NamespaceIPC, h.Cmd.Process.Pid)
		if err != nil {
			return nil, err
		}
		return f, h.Release()
	}
}

func populateNamespace(args map[string]interface{}) ([]byte, error) {
	cp, ok := args["checkpoint"].(string)
	if !ok || cp == "" {
		return nil, errors.New("checkpoint must be provided")
	}
	if err := os.Chdir(cp); err != nil {
		return nil, err
	}
	infos, err := ioutil.ReadDir(".")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read dir "+cp)
	}
	const (
		varFilePrefix = "ipcns-var-"
		shmFilePrefix = "ipcns-shm-"
		semFilePrefix = "ipcns-sem-"
		msgFilePrefix = "ipcns-msg-"
		prefixLen     = len(varFilePrefix)
	)
	for _, info := range infos {
		if len(info.Name()) > prefixLen {
			pre := info.Name()[:prefixLen]
			switch pre {
			case varFilePrefix:
				{
					if err = restoreIPCVars(info.Name()); err != nil {
						return nil, errors.Wrap(err, "failed to restore vars using "+info.Name())
					}
				}
			case shmFilePrefix:
				{
					if err = restoreIPCShm(info.Name()); err != nil {
						return nil, errors.Wrap(err, "failed to restore shm using "+info.Name())
					}
				}
			case msgFilePrefix:
			case semFilePrefix:
			default:
			}
		}
	}
	return nil, nil
}

func restoreIPCVars(file string) error {
	img, err := criuimages.New(file)
	if err != nil {
		return err
	}
	defer img.Close()
	entry := &criutype.IpcVarEntry{}
	if err = img.ReadOne(entry); err != nil {
		return err
	}
	scatter := utils.NewFieldsScatterer(entry, targets)
	if err = scatter.Scatter(); err != nil {
		return err
	}
	return nil
}

func restoreIPCShm(file string) error {
	img, err := criuimages.New(file)
	if err != nil {
		return errors.Wrap(err, "failed to open image "+file)
	}
	defer img.Close()
	for {
		entry := &criutype.IpcShmEntry{}
		if err = img.ReadOne(entry); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		nextID := strconv.FormatUint(uint64(entry.GetDesc().GetId()), 10)
		if err = utils.SysCtlWrite(kernelShmNextID, nextID); err != nil {
			return errors.Wrap(err, "failed to set shm next id to "+nextID)
		}
		key := int(entry.GetDesc().GetKey())
		size := uint64(entry.GetSize())
		mode := int(entry.GetDesc().GetMode())
		shm, err := ipcgo.NewSharedMemory(key, size, mode)
		if err != nil {
			return errors.Wrapf(err, "failed to create shm with key %v size %v mode %o", key, size, mode)
		}
		if uint32(shm.ID()) != entry.GetDesc().GetId() {
			return errors.Errorf("shm id mismatch(%d instead of %d)", shm.ID(), entry.GetDesc().GetId())
		}
		uid, gid := entry.Desc.Uid, entry.Desc.Gid
		if err = shm.SetStat(uid, gid, nil); err != nil {
			return errors.Wrapf(err, "failed to set stat with uid %v gid %v", *uid, *gid)
		}
		if err = restoreShmPages(img, entry, shm); err != nil {
			return errors.Wrap(err, "failed to restore shm pages")
		}
	}
	return nil
}

func restoreShmPages(img *criuimages.Image, entry *criutype.IpcShmEntry, shm *ipcgo.SharedMemory) (err error) {
	if err = shm.Attach(0, 0); err != nil {
		return err
	}
	defer func() {
		errClose := shm.Close()
		if err != nil {
			if errClose != nil {
				err = errors.Wrap(err, errClose.Error())
			}
		} else {
			err = errClose
		}
	}()
	if entry.GetInPagemaps() {
		return restoreFromPagemaps(int(entry.GetDesc().GetId()), shm)
	}
	// or we just read data from the image file
	file := img.File()
	expectReadSize := roundUp(entry.GetSize(), 4)
	_, err = io.CopyN(shm, file, int64(expectReadSize))
	if err != nil {
		err = errors.Wrap(err, "failed to copy data from image file to shm")
	}
	return
}

func restoreFromPagemaps(shmid int, shm *ipcgo.SharedMemory) error {
	pagemapTemplate := "pagemap-shmem-%d.img"
	pagesTemplate := "pages-%d.img"
	pagemapName := fmt.Sprintf(pagemapTemplate, shmid)
	pagemap, err := criuimages.New(pagemapName)
	if err != nil {
		return err
	}
	defer pagemap.Close()
	//read pagemap head
	head := &criutype.PagemapHead{}
	if err = pagemap.ReadOne(head); err != nil {
		return err
	}
	pagesName := fmt.Sprintf(pagesTemplate, head.GetPagesId())
	pages, err := os.Open(pagesName)
	if err != nil {
		return err
	}
	defer pages.Close()
	mapEntry := &criutype.PagemapEntry{}
	for {
		if err = pagemap.ReadOne(mapEntry); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		expectReadSize := int64(mapEntry.GetNrPages()) * pageSize
		if _, err = shm.Seek(mapEntry.GetVaddr(), io.SeekCurrent); err != nil {
			return errors.Wrapf(err, "failed to seek memory to %x", mapEntry.GetVaddr())
		}
		if _, err = io.CopyN(shm, pages, expectReadSize); err != nil {
			return err
		}
	}
	return nil
}

func devide(refs []string, defaultVars *criutype.IpcVarEntry, supplier services.CheckpointSupplier) (normals []string, specials []string, err error) {
	var cp string
	var inDefault bool
	for _, ref := range refs {
		cp, err = supplier.Get(ref)
		if err != nil {
			err = errors.Wrap(err, "failed to get checkpoint path for "+ref)
			return
		}
		inDefault, err = inDefaultNamespace(ref, defaultVars, cp)
		if err != nil {
			return
		}
		if inDefault {
			normals = append(normals, ref)
		} else {
			specials = append(specials, ref)
		}
	}
	return
}

func inDefaultNamespace(ref string, vars *criutype.IpcVarEntry, cp string) (bool, error) {
	extraFilePrefixes := dumpFileNamePrefixes[:3]
	varsFilePrefix := dumpFileNamePrefixes[3]
	var varsFileName string
	infos, err := ioutil.ReadDir(cp)
	if err != nil {
		return false, errors.Wrap(err, "failed to read dir "+cp)
	}
	for _, info := range infos {
		for _, prefix := range extraFilePrefixes {
			if strings.HasPrefix(info.Name(), prefix) {
				return false, nil
			}
		}
		if strings.HasPrefix(info.Name(), varsFilePrefix) {
			varsFileName = info.Name()
		}
	}
	if varsFileName == "" {
		return false, errors.Errorf("file with prefix %s does not exist", varsFilePrefix)
	}
	imgPath := path.Join(cp, varsFileName)
	img, err := criuimages.New(imgPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to create image")
	}
	defer img.Close()
	entry := &criutype.IpcVarEntry{}
	if err = img.ReadOne(entry); err != nil {
		return false, errors.Wrap(err, "failed to read entry")
	}
	return proto.Equal(entry, vars), nil
}

func getDefaultNamespace() (*criutype.IpcVarEntry, error) {
	h, err := namespace.NewNamespaceExecCreateHelper(functionKeyCollect, types.NamespaceIPC, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ipc create helper")
	}
	entry := &criutype.IpcVarEntry{}
	if err := h.Do(true); err != nil {
		return nil, errors.Wrap(err, "failed to run helper")
	}
	if err := proto.Unmarshal(h.Ret, entry); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal entry")
	}
	return entry, nil
}

func collect(map[string]interface{}) ([]byte, error) {
	entry := &criutype.IpcVarEntry{}
	gatherer := utils.NewFieldsGatherer(entry, sources)
	if err := gatherer.Gather(); err != nil {
		return nil, errors.Wrap(err, "failed to gather fields")
	}
	data, err := proto.Marshal(entry)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal entry")
	}
	return data, nil
}

func roundUp(num, multiple uint64) uint64 {
	return ((num + multiple - 1) / multiple) * multiple
}
