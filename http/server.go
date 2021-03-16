package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/client"
	"github.com/YLonely/cer-manager/log"
	ns "github.com/YLonely/cer-manager/namespace"
	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

func init() {
	ns.PutNamespaceFunction(namespaceFunctionKeyHealthCheck, types.NamespaceNET, healthCheck)
}

func NewServer(root string, port int) (*Server, error) {
	rootPath := path.Join(root, "http")
	if err := os.MkdirAll(rootPath, 0666); err != nil {
		return nil, err
	}
	s := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%d", port),
	}
	ret := &Server{
		root: rootPath,
		s:    s,
	}
	http.HandleFunc("/namespace/update", ret.updateNamespace)
	http.HandleFunc("/image/upload", ret.uploadImage)
	http.HandleFunc("/checkpoint", ret.makeCheckpoint)
	return ret, nil
}

const (
	// 5GB
	defaultMaximumFileSize          = 5 << 30
	defaultContainerdAddress        = "/run/containerd/containerd.sock"
	namespaceFunctionKeyHealthCheck = "check"
)

type Server struct {
	root string
	s    *http.Server
}

func (svr *Server) Start() chan error {
	errorC := make(chan error, 1)
	go func() {
		log.Logger(cerm.HttpService, "Start").Info("Service started")
		if err := svr.s.ListenAndServe(); err != http.ErrServerClosed {
			errorC <- err
		}
	}()
	return errorC
}

func (svr *Server) Shutdown() error {
	if err := svr.s.Shutdown(context.Background()); err != nil {
		return err
	}
	return nil
}

func (svr *Server) updateNamespace(w http.ResponseWriter, r *http.Request) {
	entry := log.Logger(cerm.HttpService, "updateNamespace")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	c, err := client.Default()
	if err != nil {
		entry.WithError(err).Error("failed to create cer-manager client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer c.Close()
	req := updateNamespaceRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.CheckpointName == "" {
		http.Error(w, "checkpoint name can not be empty", http.StatusBadRequest)
		return
	}
	resp := updateNamespaceResponse{}
	err = c.UpdateNamespace(types.NewContainerdReference(req.CheckpointName, req.CheckpointNamespace), req.Capacity)
	if err != nil {
		resp.Message = err.Error()
	} else {
		resp.Message = "OK"
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		entry.Error(err)
	}
}

func (svr *Server) uploadImage(w http.ResponseWriter, r *http.Request) {
	entry := log.Logger(cerm.HttpService, "uploadImage")
	r.ParseMultipartForm(defaultMaximumFileSize)
	file, handler, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		entry.WithError(err).Error("failed to parse file from the form")
		return
	}
	defer file.Close()
	entry.Infof("receive a file %s with size %v", handler.Filename, handler.Size)
	uploadPath := path.Join(svr.root, "uploads")
	if err := os.MkdirAll(uploadPath, 0666); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		entry.WithError(err).Error("failed to create the uploads folder")
		return
	}
	dest, err := os.Create(path.Join(uploadPath, handler.Filename))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		entry.WithError(err).Error("failed to create the destination file")
		return
	}
	if _, err := io.Copy(dest, file); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		entry.WithError(err).Error("failed to write the destination file")
	}
	fmt.Fprintf(w, "uploading file successfully\n")
}

func (svr *Server) makeCheckpoint(w http.ResponseWriter, r *http.Request) {
	entry := log.Logger(cerm.HttpService, "makeCheckpoint")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	req := makeCheckpointRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.TarFileName == "" || req.ImageName == "" || req.CheckpointName == "" {
		http.Error(w, "tar file name, image name or checkpoint name can not be empty", http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	tarFilePath := path.Join(svr.root, "uploads", req.TarFileName)
	if _, err := os.Stat(tarFilePath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("image %s does not exist", req.ImageName), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		entry.WithError(err).Error("failed to stat image")
	}
	ctx, client, err := initial(&req)
	if err != nil {
		entry.Error(err)
		http.Error(w, errInitialFailed.Error(), http.StatusInternalServerError)
		return
	}
	img, err := importImage(ctx, client, tarFilePath, &req)
	if err != nil {
		entry.Error(err)
		http.Error(w, errImageImportFailed.Error(), http.StatusInternalServerError)
		return
	}
	c, err := startContainer(ctx, client, img)
	if err != nil {
		entry.Error(err)
		http.Error(w, errContainerStartFailed.Error(), http.StatusInternalServerError)
		return
	}
	err = checkpoint(ctx, client, c, &req)
	if err != nil {
		entry.Error(err)
		http.Error(w, errCheckpointFailed.Error(), http.StatusInternalServerError)
	}
}

func initial(req *makeCheckpointRequest) (ctx context.Context, client *containerd.Client, err error) {
	var (
		ps string
		pt v1.Platform
	)
	ps = platforms.DefaultString()
	pt, err = platforms.Parse(ps)
	if err != nil {
		return
	}
	client, err = containerd.New(defaultContainerdAddress, containerd.WithDefaultPlatform(platforms.Only(pt)))
	if err != nil {
		return
	}
	ctx = namespaces.WithNamespace(context.Background(), req.Namespace)
	return
}

func importImage(ctx context.Context, client *containerd.Client, tarFilePath string, req *makeCheckpointRequest) (*images.Image, error) {
	reader, err := os.Open(tarFilePath)
	if err != nil {
		return nil, err
	}
	imgs, err := client.Import(ctx, reader, containerd.WithAllPlatforms(false))
	if err != nil {
		return nil, err
	}
	img := containerd.NewImage(client, imgs[0])
	if err = img.Unpack(ctx, ""); err != nil {
		return nil, err
	}
	cctx, done, err := client.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(cctx)
	imageService := client.ImageService()
	oldName := imgs[0].Name
	imgs[0].Name = req.ImageName
	if _, err = imageService.Create(cctx, imgs[0]); err != nil {
		if errdefs.IsAlreadyExists(err) {
			if err = imageService.Delete(cctx, req.ImageName); err != nil {
				return nil, err
			}
			if _, err = imageService.Create(cctx, imgs[0]); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if err = imageService.Delete(cctx, oldName); err != nil {
		return nil, err
	}
	return &imgs[0], nil
}

func startContainer(ctx context.Context, client *containerd.Client, img *images.Image) (c containerd.Container, err error) {
	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
		s     specs.Spec
	)
	id := generateID(*img)
	image := containerd.NewImage(client, *img)
	opts = append(opts, oci.WithDefaultSpec(), oci.WithDefaultUnixDevices, oci.WithImageConfig(image))
	cOpts = append(cOpts, containerd.WithImage(image), containerd.WithSpec(&s, opts...))
	c, err = client.NewContainer(ctx, id, cOpts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			c.Delete(ctx)
		}
	}()
	var con console.Console
	task, err := tasks.NewTask(ctx, client, c, "", con, false, "", []cio.Opt{})
	if err != nil {
		return nil, err
	}
	if err = task.Start(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			task.Delete(ctx, containerd.WithProcessKill)
		}
	}()
	helper, err := ns.NewNamespaceExecEnterHelper(
		namespaceFunctionKeyHealthCheck,
		types.NamespaceNET,
		fmt.Sprintf("/proc/%d/ns/net", task.Pid()),
		nil,
	)
	if err != nil {
		return nil, err
	}
	if err = helper.Do(true); err != nil {
		return nil, errors.Wrap(err, "failed to do health check")
	}
	if string(helper.Ret) != "OK" {
		return nil, errors.Errorf("health check returns %s", string(helper.Ret))
	}
	return
}

func checkpoint(ctx context.Context, client *containerd.Client, c containerd.Container, req *makeCheckpointRequest) error {
	defer func() {
		c.Delete(ctx)
	}()
	opts := []containerd.CheckpointOpts{
		containerd.WithCheckpointRuntime,
		containerd.WithCheckpointImage,
		containerd.WithCheckpointRW,
		containerd.WithCheckpointTask,
	}
	task, err := c.Task(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		task.Delete(ctx, containerd.WithProcessKill)
	}()
	if err := task.Pause(ctx); err != nil {
		return err
	}
	if _, err := c.Checkpoint(ctx, req.CheckpointName, opts...); err != nil {
		return err
	}
	return nil
}

func healthCheck(map[string]interface{}) ([]byte, error) {
	var (
		round  = 10
		period = time.Millisecond * 500
		last   error
	)
	for i := 0; i < round; i++ {
		resp, err := http.Get("http://127.0.0.1:8080/_/health/")
		if err == nil {
			defer resp.Body.Close()
			bs, err := ioutil.ReadAll(resp.Body)
			if err == nil && strings.Contains(string(bs), "OK") {
				return []byte("OK"), nil
			} else {
				last = err
			}
		} else {
			last = err
		}
		time.Sleep(period)
	}
	if last == nil {
		last = errors.New("health check timeout")
	}
	return nil, last
}

func generateID(img images.Image) string {
	bs, err := json.Marshal(img)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(bs))
}
