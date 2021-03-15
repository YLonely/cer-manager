package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	gohttp "net/http"
	"os"
	"path"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/client"
	"github.com/YLonely/cer-manager/log"
)

func NewServer(root string, port int) (*Server, error) {
	rootPath := path.Join(root, "http")
	if err := os.MkdirAll(rootPath, 0666); err != nil {
		return nil, err
	}
	s := &gohttp.Server{
		Addr: fmt.Sprintf("0.0.0.0:%d", port),
	}
	ret := &Server{
		root: rootPath,
		s:    s,
	}
	http.HandleFunc("/namespace/update", ret.updateNamespace)
	http.HandleFunc("/image/upload", ret.uploadImage)
	return ret, nil
}

const (
	// 5GB
	defaultMaximumFileSize = 5 << 30
)

type Server struct {
	root string
	s    *gohttp.Server
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

func (svr *Server) updateNamespace(w gohttp.ResponseWriter, r *gohttp.Request) {
	if r.Method != gohttp.MethodPost {
		w.WriteHeader(gohttp.StatusMethodNotAllowed)
		return
	}
	c, err := client.Default()
	if err != nil {
		log.Logger(cerm.HttpService, "updateNamespace").WithError(err).Error("failed to create cer-manager client")
		w.WriteHeader(gohttp.StatusInternalServerError)
		return
	}
	req := updateNamespaceRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(gohttp.StatusBadRequest)
		return
	}
	resp := updateNamespaceResponse{}
	err = c.UpdateNamespace(types.NewContainerdReference(req.CheckpointName, req.CheckpointNamespace), req.Capacity)
	if err != nil {
		resp.Message = err.Error()
	} else {
		resp.Message = "OK"
	}
	w.WriteHeader(gohttp.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Logger(cerm.HttpService, "updateNamespace").Error(err)
	}
}

func (svr *Server) uploadImage(w gohttp.ResponseWriter, r *gohttp.Request) {
	r.ParseMultipartForm(defaultMaximumFileSize)
	file, handler, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(gohttp.StatusInternalServerError)
		log.Logger(cerm.HttpService, "uploadImage").WithError(err).Error("failed to parse file from the form")
		return
	}
	defer file.Close()
	log.Logger(cerm.HttpService, "uploadImage").Infof("receive a file %s with size %v", handler.Filename, handler.Size)
	uploadPath := path.Join(svr.root, "uploads")
	if err := os.MkdirAll(uploadPath, 0666); err != nil {
		w.WriteHeader(gohttp.StatusInternalServerError)
		log.Logger(cerm.HttpService, "uploadImage").WithError(err).Error("failed to create the uploads folder")
		return
	}
	dest, err := os.Create(path.Join(uploadPath, handler.Filename))
	if err != nil {
		w.WriteHeader(gohttp.StatusInternalServerError)
		log.Logger(cerm.HttpService, "uploadImage").WithError(err).Error("failed to create the destination file")
		return
	}
	if _, err := io.Copy(dest, file); err != nil {
		w.WriteHeader(gohttp.StatusInternalServerError)
		log.Logger(cerm.HttpService, "uploadImage").WithError(err).Error("failed to write the destination file")
	}
	fmt.Fprintf(w, "uploading file successfully\n")
}
