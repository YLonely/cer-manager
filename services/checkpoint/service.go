package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"

	api "github.com/YLonely/cer-manager/api/services/checkpoint"
	cp "github.com/YLonely/cer-manager/checkpoint"
	"github.com/YLonely/cer-manager/checkpoint/ccfs"
	"github.com/YLonely/cer-manager/checkpoint/containerd"
	"github.com/YLonely/cer-manager/utils"

	"path"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/services"
	"github.com/pkg/errors"
)

func New(root string) (services.Service, error) {
	const configName = "checkpoint_service.json"
	content, err := ioutil.ReadFile(path.Join(root, configName))
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}
	c := config{}
	if json.Unmarshal(content, &c); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config file")
	}
	p, err := initProvider(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create provider")
	}
	return &service{
		c:        c,
		root:     path.Join(root, "checkpoint"),
		router:   services.NewRouter(),
		targets:  map[string]struct{}{},
		provider: p,
	}, nil
}

type service struct {
	c        config
	root     string
	router   services.Router
	provider cp.Provider
	//targets records all the target path where the checkpoint files located
	targets map[string]struct{}
	m       sync.Mutex
}

type config struct {
	// ccfs or containerd
	Type string `json:"type"`
	// example: localhost:5000
	Registry string `json:"registry"`
}

var _ services.Service = &service{}

func (s *service) Init() error {
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return err
	}
	s.router.AddHandler(api.MethodGetCheckpoint, s.handleGetCheckpoint)
	log.Logger(cerm.CheckpointService, "Init").Info("Service initialized")
	return nil
}

func (s *service) Handle(ctx context.Context, c net.Conn) {
	if err := s.router.Handle(c); err != nil {
		log.Logger(cerm.CheckpointService, "").Error(err.Error())
		c.Close()
	}
}

func (s *service) Stop() error {
	var failed []string
	for t := range s.targets {
		if err := s.provider.Remove(t); err != nil {
			failed = append(failed, fmt.Sprintf("remove %s with error %s", t, err.Error()))
		}
	}
	if len(failed) != 0 {
		return errors.New(strings.Join(failed, ";"))
	}
	return nil
}

func (s *service) Get(ref string) (string, error) {
	if ref == "" {
		return "", errors.New("empty ref")
	}
	target := path.Join(s.root, ref)
	if _, exists := s.targets[target]; exists {
		return target, nil
	}
	s.m.Lock()
	defer s.m.Unlock()
	if _, exists := s.targets[target]; exists {
		return target, nil
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return "", errors.Wrap(err, "failed to create dir "+target)
	}
	if err := s.provider.Prepare(ref, target); err != nil {
		return "", err
	}
	s.targets[target] = struct{}{}
	return target, nil
}

func (s *service) handleGetCheckpoint(c net.Conn) error {
	var r api.GetCheckpointRequest
	if err := utils.ReceiveObject(c, &r); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.CheckpointService, "GetCheckpoint"), "request", r).Info()
	var resp api.GetCheckpointResponse
	var err error
	resp.Path, err = s.Get(r.Ref)
	if err != nil {
		log.Logger(cerm.CheckpointService, "GetCheckpoint").Error(err.Error())
	}
	if err := utils.SendObject(c, resp); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.CheckpointService, "GetCheckpoint"), "response", resp).Info()
	return nil
}

func initProvider(c config) (cp.Provider, error) {
	var p cp.Provider
	var err error
	switch c.Type {
	case "ccfs":
		if c.Registry == "" {
			return nil, errors.New("empty registry")
		}
		p, err = ccfs.NewProvider(c.Registry)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ccfs provider")
		}
	case "containerd":
		p, err = containerd.NewProvider()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create containerd provider")
		}
	default:
		return nil, errors.New("invalid provider type")
	}
	return p, nil
}
