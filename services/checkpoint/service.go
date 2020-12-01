package checkpoint

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/mount"
	"github.com/YLonely/cer-manager/services"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func New(root string) (services.Service, error) {
	const configName = "checkpoint_service.json"
	content, err := ioutil.ReadFile(path.Join(root, configName))
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}
	c := &config{}
	if json.Unmarshal(content, c); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config file")
	}
	if c.Registry == "" {
		return nil, errors.New("empty registry")
	}
	return &service{
		registry: c.Registry,
		root:     path.Join(root, "checkpoint"),
	}, nil
}

type service struct {
	registry string
	root     string
}

type config struct {
	// example: localhost:5000
	Registry string `json:"registry"`
}

var _ services.Service = &service{}

func (s *service) Init() error {
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return err
	}
	// TODO: mount cfs on s.root
	log.Logger(services.CheckpointService, "Init").Info("Service initialized")
	return nil
}

func (s *service) Handle(ctx context.Context, c net.Conn) {

}

func (s *service) Stop() error {
	return mount.Unmount(s.root, unix.MNT_DETACH)
}
