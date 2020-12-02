package services

import (
	"net"

	"github.com/YLonely/cer-manager/utils"
	"github.com/pkg/errors"
)

type Handler func(net.Conn) error

type Router struct {
	hs map[string]Handler
}

func NewRouter() Router {
	return Router{
		hs: map[string]Handler{},
	}
}

func (r Router) Handle(c net.Conn) error {
	var method string
	err := utils.ReceiveObject(c, &method)
	if err != nil {
		return err
	}
	handler, exists := r.hs[method]
	if !exists {
		return errors.New("no matched hanlder")
	}
	return handler(c)
}

func (r *Router) AddHandler(method string, h Handler) {
	r.hs[method] = h
}
