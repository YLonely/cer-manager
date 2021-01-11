package ccfs

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/YLonely/ccfs/cache"
	"github.com/YLonely/cer-manager/checkpoint"
	"github.com/YLonely/cer-manager/log"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

type Config struct {
	Exec                      string `json:"exec,omitempty"`
	CacheDirectory            string `json:"cache_directory,omitempty"`
	Registry                  string `json:"registry"`
	CacheEntriesPerCheckpoint int    `json:"cache_entries_per_checkpoint,omitempty"`
	MaxCacheEntries           int    `json:"max_cache_entries,omitempty"`
	GCInterval                int    `json:"gc_interval,omitempty"`
}

// NewProvider returns a provider based on ccfs
func NewProvider(c Config) (checkpoint.Provider, func() error, error) {
	var err error
	cachePath, mountPath := path.Join(defaultCCFSRoot, "cache"), path.Join(defaultCCFSRoot, "mountpoint")
	if c.CacheDirectory != "" {
		cachePath = c.CacheDirectory
	} else {
		c.CacheDirectory = cachePath
	}
	if c.Exec == "" {
		c.Exec = "ccfs"
	}
	if err = unix.Unmount(mountPath, unix.MNT_DETACH); err != nil && err != syscall.EINVAL && err != syscall.ENOENT {
		return nil, nil, errors.Wrap(err, "failed to umount ccfs")
	}
	if err = os.MkdirAll(cachePath, 0644); err != nil {
		return nil, nil, err
	}
	if err = os.MkdirAll(mountPath, 0644); err != nil {
		return nil, nil, err
	}
	var done func() error
	if done, err = mountCCFS(mountPath, c); err != nil {
		return nil, nil, errors.Wrap(err, "failed to mount ccfs")
	}
	p := &provider{
		mountpoint: mountPath,
		refs:       map[string]int{},
		lastRefs:   map[string]int{},
		config:     c,
	}
	go p.scan()
	return p, done, nil
}

const (
	defaultCCFSRoot  = "/tmp/.ccfs"
	mountsInfo       = "/proc/mounts"
	ccfsStateFile    = ".end"
	ccfsWeightFile   = ".weight"
	ccfsStateValid   = "valid"
	ccfsStateInvalid = "invalid"
	waitInterval     = time.Millisecond * 50
	scanInterval     = time.Second * 5
)

var _ checkpoint.Provider = &provider{}

type provider struct {
	mountpoint string
	mu         sync.Mutex
	// refs records the reference counts on different checkpoint names
	refs     map[string]int
	lastRefs map[string]int
	config   Config
}

func (p *provider) Prepare(checkpointName string, target string) (err error) {
	checkpointDir := path.Join(p.mountpoint, checkpointName)
	if _, err = os.Stat(checkpointDir); err == nil {
		return nil
	}
	// we make a dir named ref and the ccfs will do the rest of the work
	if err = os.Mkdir(checkpointDir, 0644); err != nil {
		err = errors.Wrap(err, "failed to create dir "+checkpointDir)
		return
	}
	defer func() {
		if err != nil {
			os.RemoveAll(checkpointDir)
		}
	}()
	statPath := path.Join(checkpointDir, ccfsStateFile)
	for i := 0; i < 5; i++ {
		time.Sleep(waitInterval)
		if _, err = os.Stat(statPath); err != nil {
			if !os.IsNotExist(err) {
				return
			}
		} else {
			break
		}
	}
	if err != nil {
		err = errors.New("wait ccfs time out")
		return
	}
	var stat []byte
	stat, err = ioutil.ReadFile(statPath)
	if err != nil {
		return
	}
	switch string(stat) {
	case ccfsStateValid:
	case ccfsStateInvalid:
		return errors.Errorf("invalid ccfs dir, ccfs log file at %s", path.Join(defaultCCFSRoot, "ccfs.log"))
	default:
		return errors.New("unknown ccfs state type")
	}
	if err = unix.Mount(checkpointDir, target, "", unix.MS_BIND, ""); err != nil {
		err = errors.Wrap(err, "failed to bind mount to target path")
	}
	return
}

func (p *provider) Remove(target string) error {
	return unix.Unmount(target, unix.MNT_DETACH)
}

func (p *provider) Add(checkpointName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refs[checkpointName]++
}

func (p *provider) Release(checkpointName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refs[checkpointName]--
	if p.refs[checkpointName] < 0 {
		p.refs[checkpointName] = 0
	}
}

func (p *provider) scan() {
	ticker := time.NewTicker(scanInterval)
	changedCheckpoint := map[string]struct{}{}
	for {
		p.mu.Lock()
		for name, refCount := range p.refs {
			if p.lastRefs[name] != refCount {
				p.lastRefs[name] = refCount
				changedCheckpoint[name] = struct{}{}
			}
		}
		p.mu.Unlock()
		for name := range changedCheckpoint {
			dynamicWeight := p.lastRefs[name]
			weightFilePath := path.Join(p.mountpoint, name, ccfsWeightFile)
			data, err := ioutil.ReadFile(weightFilePath)
			if err != nil {
				log.Raw().WithError(err).Warnf("failed to read weight file %s", weightFilePath)
				continue
			}
			weightStr := strings.Trim(string(data), " \n\t")
			parts := strings.Split(weightStr, ",")
			if len(parts) != 2 {
				log.Raw().Errorf("weight file %s has invalid content %q", weightFilePath, weightStr)
				continue
			}
			parts[1] = strconv.Itoa(dynamicWeight)
			weightStr = strings.Join(parts, ",")
			if err = ioutil.WriteFile(weightFilePath, []byte(weightStr), 0); err != nil {
				log.Raw().WithError(err).Errorf("failed to write file %s", weightFilePath)
			}
			// if no error occurs, we can delete the name from map
			delete(changedCheckpoint, name)
		}
		<-ticker.C
	}
}

func mountCCFS(mountPath string, c Config) (func() error, error) {
	cacheConfig := cache.Config{
		Directory:              c.CacheDirectory,
		Level1MaxLRUCacheEntry: c.CacheEntriesPerCheckpoint,
		MaxLRUCacheEntry:       c.MaxCacheEntries,
		GCInterval:             c.GCInterval,
	}
	data, err := json.MarshalIndent(cacheConfig, "", "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal cache config")
	}
	err = ioutil.WriteFile(path.Join(defaultCCFSRoot, "cache-config.json"), data, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write cache config")
	}
	cmd := exec.Command(
		c.Exec,
		"--debug",
		"--config",
		path.Join(defaultCCFSRoot, "cache-config.json"),
		c.Registry,
		mountPath,
	)
	logf, err := os.OpenFile(path.Join(defaultCCFSRoot, "ccfs.log"), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create log file for ccfs")
	}
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err = cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start ccfs")
	}
	done := func() error {
		defer logf.Close()
		if err := unix.Unmount(mountPath, unix.MNT_DETACH); err != nil {
			return errors.Wrap(err, "failed to unmount ccfs")
		}
		cmd.Process.Kill()
		cmd.Wait()
		return nil
	}
	return done, nil
}
