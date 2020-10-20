package utils

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"

	"github.com/YLonely/cr-daemon/service"
)

const (
	dataSizePrefixLen int    = 4
	dataSizeMax       uint32 = math.MaxUint32
)

/*
	serviceType     methodLen      method      requestLen      request
   |___1byte__|_______4byte______|_________|_______4byte_____|__________|
*/
func Pack(st service.ServiceType, method string, request interface{}) ([]byte, error) {
	svrTypeBinary, err := WriteBinary(st)
	if err != nil {
		return nil, err
	}

}

//WriteBinary writes value to bytes
func WriteBinary(v interface{}) ([]byte, error) {
	data := new(bytes.Buffer)
	if err := binary.Write(data, binary.BigEndian, v); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

//WithSizePrefix packs v with prefix of data size
func WithSizePrefix(v interface{}) error {
	dataJson, err := json.Marshal(v)
	if err != nil {
		return err
	}
	l := uint32(len(data))
	if l > dataSizeMax {
		return errors.New("data size is out of range")
	}
	dataPrefix, err := WriteBinary(l)
	if err != nil {
		return err
	}
	data = append(dataPrefix, data...)
	if n, err := c.Write(data); err != nil || n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

//Receive data with size prefix
func Receive(c net.Conn) ([]byte, error) {
	var l uint32
	dataPrefix := make([]byte, dataSizePrefixLen)
	if _, err := io.ReadFull(c, dataPrefix); err != nil {
		return nil, err
	}
	l = binary.BigEndian.Uint32(dataPrefix)
	data := make([]byte, l)
	if _, err := io.ReadFull(c, data); err != nil {
		return nil, err
	}
	return data, nil
}
