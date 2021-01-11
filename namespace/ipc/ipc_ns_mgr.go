package ipc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/log"
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

// NewManager returns a new ipc namespace manager
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
	log.Raw().Debugf("images with normal ipc %v, images with special ipc %v", normals, specials)
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
	maxMsgSize                                        = 8192
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
		// we try to get a normal type of ipc for ref
		mgr, exists = m.managers[ipcTypeNormal]
		if exists {
			fd, info, err = mgr.Get(nil)
			if err != nil {
				err = errors.Wrap(err, "failed to get a normal ipc")
				return
			}
		} else {
			err = errors.Errorf("ipc namespace of %s does not exist", ref)
			return
		}
	} else {
		fd, info, err = mgr.Get(nil)
		if err != nil {
			err = errors.Wrap(err, "failed to get namespace for "+ref)
			return
		}
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
				if err = restoreIPCVars(info.Name()); err != nil {
					return nil, errors.Wrap(err, "failed to restore vars using "+info.Name())
				}
			case shmFilePrefix:
				if err = restoreIPCShm(info.Name()); err != nil {
					return nil, errors.Wrap(err, "failed to restore shm using "+info.Name())
				}
			case msgFilePrefix:
				if err = restoreIPCMsg(info.Name()); err != nil {
					return nil, errors.Wrap(err, "failed to restore msg using "+info.Name())
				}
			case semFilePrefix:
				if err = restoreIPCSem(info.Name()); err != nil {
					return nil, errors.Wrap(err, "failed to restore sem using "+info.Name())
				}
			default:
			}
		}
	}
	return nil, nil
}

func restoreIPCSem(file string) error {
	img, err := criuimages.New(file)
	if err != nil {
		return err
	}
	defer img.Close()
	entry := &criutype.IpcSemEntry{}
	for {
		err = img.ReadOne(entry)
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "failed to read sem entry")
		}
		str := strconv.FormatUint(uint64(entry.GetDesc().GetId()), 10)
		if err = utils.SysCtlWrite(kernelSemNextID, str); err != nil {
			return errors.Wrap(err, "failed to write sem next id")
		}
		semSet, err := ipcgo.NewSemaphoreSet(int(entry.GetDesc().GetKey()), int(entry.GetNsems()), int(entry.GetDesc().GetMode()))
		if err != nil {
			return errors.Wrap(err, "failed to create a new semaphore set")
		}
		if semSet.ID() != int(entry.GetDesc().GetId()) {
			return errors.Errorf("failed to restore sem id (%d instead of %d)", int(entry.GetDesc().GetId()), semSet.ID())
		}
		if err = semSet.SetStat(entry.GetDesc().Uid, entry.GetDesc().Gid, nil); err != nil {
			return errors.New("failed to set stat")
		}
		if err = restoreSemValues(img, entry, semSet); err != nil {
			return errors.Wrap(err, "failed to restore semaphore values")
		}
	}
	return nil
}

func restoreSemValues(img *criuimages.Image, entry *criutype.IpcSemEntry, semSet *ipcgo.SemaphoreSet) error {
	size := roundUp(uint64(2*entry.GetNsems()), 8)
	bs := make([]byte, size)
	file := img.File()
	if _, err := io.ReadFull(file, bs); err != nil {
		return errors.Wrap(err, "failed to read data from sem image")
	}
	values := make([]uint16, int(entry.GetNsems()))
	buffer := bytes.NewBuffer(bs)
	var tmp uint16
	for i := range values {
		if err := binary.Read(buffer, binary.LittleEndian, &tmp); err != nil {
			return errors.Wrap(err, "failed to parse bytes data")
		}
		values[i] = tmp
	}
	if err := semSet.SetAll(values); err != nil {
		return err
	}
	return nil
}

func restoreIPCMsg(file string) error {
	img, err := criuimages.New(file)
	if err != nil {
		return err
	}
	defer img.Close()
	entry := &criutype.IpcMsgEntry{}
	for {
		err = img.ReadOne(entry)
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "failed to read msg entry")
		}
		str := strconv.FormatUint(uint64(entry.GetDesc().GetId()), 10)
		if err = utils.SysCtlWrite(kernelMsgNextID, str); err != nil {
			return errors.Wrap(err, "failed to write message next id")
		}
		mq, err := ipcgo.NewMessageQueue(int(entry.GetDesc().GetKey()), int(entry.GetDesc().GetMode()))
		if err != nil {
			return errors.Wrap(err, "failed to create a new message queue")
		}
		if mq.ID() != int(entry.GetDesc().GetId()) {
			return errors.Errorf("failed to restore message id (%d instead of %d)", entry.GetDesc().GetId(), mq.ID())
		}
		if err = mq.SetStat(entry.GetDesc().Uid, entry.GetDesc().Gid, nil, nil); err != nil {
			return errors.Wrap(err, "failed to set stat of the message queue")
		}
		if err = restoreMessages(img, entry, mq); err != nil {
			return errors.Wrap(err, "failed to restore messages")
		}
	}
	return nil
}

func restoreMessages(img *criuimages.Image, entry *criutype.IpcMsgEntry, mq *ipcgo.MessageQueue) error {
	msg := &criutype.IpcMsg{}
	for i := 0; i < int(entry.GetQnum()); i++ {
		err := img.ReadOne(msg)
		if err != nil {
			return err
		}
		if msg.GetMsize() > maxMsgSize {
			return errors.Errorf("unsupported message size: %d", msg.GetMsize())
		}
		m := &ipcgo.Message{
			MType: int64(msg.GetMtype()),
			MText: make([]byte, int(roundUp(uint64(msg.GetMsize()), 8))),
		}
		file := img.File()
		if _, err = io.ReadFull(file, m.MText); err != nil {
			return errors.Wrap(err, "failed to read message text")
		}
		if err = mq.Send(m, ipcgo.IPC_NOWAIT); err != nil {
			return errors.Wrap(err, "failed to send message to message queue")
		}
	}
	return nil
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
	scatterer := utils.NewFieldsScatterer(entry, targets)
	if err = scatterer.Scatter(); err != nil {
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
	defer shm.Close()
	if entry.GetInPagemaps() {
		err = restoreFromPagemaps(int(entry.GetDesc().GetId()), shm)
		return
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
