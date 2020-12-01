package mux

import (
	"net"

	"github.com/YLonely/cer-manager/services"
	"github.com/YLonely/cer-manager/utils"
	"github.com/pkg/errors"
)

type Handler func(net.Conn) error

type Mux struct {
	hs map[string]Handler
	s  services.ServiceType
}

func New(s services.ServiceType) Mux {
	return Mux{
		s: s,
	}
}

func (m Mux) Handle(c net.Conn) error {
	var method string
	err := utils.ReceiveData(c, &method)
	if err != nil {
		return err
	}
	handler, exists := m.hs[method]
	if !exists {
		return errors.New("no matched hanlder")
	}
	return handler(c)
}

func (m *Mux) AddHandler(method string, h Handler) {
	m.hs[method] = h
}
