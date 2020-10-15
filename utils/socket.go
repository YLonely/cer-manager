package utils

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
)

const (
	dataSizePrefixLen int    = 4
	dataSizeMax       uint32 = math.MaxUint32
)

//Send data with prefix of data size
func Send(c net.Conn, data []byte) error {
	l := uint32(len(data))
	if l > dataSizeMax {
		return errors.New("data size is out of range")
	}
	dataPrefix := new(bytes.Buffer)
	binary.Write(dataPrefix, binary.BigEndian, l)
	data = append(dataPrefix.Bytes(), data...)
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
