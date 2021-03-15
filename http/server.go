package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	gohttp "net/http"
	"os"
	"path"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/client"
	"github.com/YLonely/cer-manager/log"
)

type updateNamespaceRequest struct {
	CheckpointName      string `json:"checkpoint_name"`
	CheckpointNamespace string `json:"checkpoint_namespace"`
	Capacity            int    `json:"capacity"`
}

type updateNamespaceResponse struct {
	Message string `json:"message"`
}

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
	return ret, nil
}

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
