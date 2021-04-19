package json

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
)

type Processor struct {
	msgHeaderLen  uint32
	littleEndian  bool
	maxPayloadLen uint32
	minPayloadLen uint32
}

func NewProcessor() *Processor {
	p := new(Processor)
	p.msgHeaderLen = 16
	p.littleEndian = false
	p.minPayloadLen = 0
	p.maxPayloadLen = 1024
	return p
}

// goroutine safe
func (p *Processor) Unmarshal(t reflect.Type, data []byte) (interface{}, error) {
	msg := reflect.New(t.Elem()).Interface()
	return msg, json.Unmarshal(data, msg)
}

// goroutine safe
func (p *Processor) Marshal(cmd uint32, msg interface{}) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal payload failed:%v", err)
	}
	pkgBuff := make([]byte, 16+len(data))
	// assemble cmd
	if p.littleEndian {
		binary.LittleEndian.PutUint32(pkgBuff[:4], cmd)
	} else {
		binary.BigEndian.PutUint32(pkgBuff[:4], cmd)
	}

	// assemble lenght
	if p.littleEndian {
		binary.LittleEndian.PutUint32(pkgBuff[4:8], uint32(16+len(data)))
	} else {
		binary.BigEndian.PutUint32(pkgBuff[4:8], uint32(16+len(data)))
	}
	copy(pkgBuff[16:], data)
	return pkgBuff, nil
}

func (p *Processor) ParseHeader(header []byte) (uint32, uint32, error) {
	if len(header) != int(p.msgHeaderLen) {
		return 0, 0, fmt.Errorf("invalid message header len,want=%v, real=%v", p.msgHeaderLen, len(header))
	}

	// parse cmd
	var cmd uint32
	if p.littleEndian {
		cmd = uint32(binary.LittleEndian.Uint32(header[:4]))
	} else {
		cmd = uint32(binary.BigEndian.Uint32(header[:4]))
	}

	// parse payload lenght
	var payloadLen uint32
	if p.littleEndian {
		payloadLen = uint32(binary.LittleEndian.Uint32(header[4:8]))
	} else {
		payloadLen = uint32(binary.BigEndian.Uint32(header[4:8]))
	}

	return payloadLen - p.msgHeaderLen, cmd, nil
}

func (p *Processor) GetHeaderLen() uint32 {
	return p.msgHeaderLen
}

func (p *Processor) GetMaxPayloadLen() uint32 {
	return p.maxPayloadLen
}

func (p *Processor) GetMinPayloadLen() uint32 {
	return p.minPayloadLen
}
