package utils

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"

	cerm "github.com/YLonely/cer-manager"
)

const (
	dataSizePrefixLen int    = 4
	dataSizeMax       uint32 = math.MaxUint32
)

/*
	serviceType     methodLen      method      requestLen      request
   |___1byte__|_______4byte______|_________|_______4byte_____|__________|
*/
func Pack(st cerm.ServiceType, method string, request interface{}) ([]byte, error) {
	svrTypeBinary, err := WriteBinary(st)
	if err != nil {
		return nil, err
	}
	methodBinary, err := WithSizePrefix(method)
	if err != nil {
		return nil, err
	}
	requestBinary, err := WithSizePrefix(request)
	if err != nil {
		return nil, err
	}
	return append(svrTypeBinary, append(methodBinary, requestBinary...)...), nil
}

//WriteBinary writes value to bytes
func WriteBinary(v interface{}) ([]byte, error) {
	data := new(bytes.Buffer)
	if err := binary.Write(data, binary.BigEndian, v); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func SendObject(conn net.Conn, v interface{}) error {
	data, err := WithSizePrefix(v)
	if err != nil {
		return err
	}
	if err = Send(conn, data); err != nil {
		return err
	}
	return nil
}

//WithSizePrefix packs v with prefix of data size
func WithSizePrefix(v interface{}) ([]byte, error) {
	dataJSON, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	l := uint32(len(dataJSON))
	if l > dataSizeMax {
		return nil, errors.New("data size is out of range")
	}
	dataPrefix, err := WriteBinary(l)
	if err != nil {
		return nil, err
	}
	data := append(dataPrefix, dataJSON...)
	return data, nil
}

//Send sends data to a conn
func Send(c net.Conn, data []byte) error {
	if n, err := c.Write(data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

//Receive data with size prefix
func ReceiveObject(c net.Conn, v interface{}) error {
	var l uint32
	dataPrefix := make([]byte, dataSizePrefixLen)
	if _, err := io.ReadFull(c, dataPrefix); err != nil {
		return err
	}
	l = binary.BigEndian.Uint32(dataPrefix)
	data := make([]byte, l)
	if _, err := io.ReadFull(c, data); err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func ReceiveServiceType(c net.Conn) (cerm.ServiceType, error) {
	data := make([]byte, cerm.ServiceTypePrefixLen)
	if _, err := io.ReadFull(c, data); err != nil {
		return 0, err
	}
	return cerm.ServiceType(binary.BigEndian.Uint16(data)), nil
}
