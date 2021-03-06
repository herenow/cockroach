// Copyright 2013 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"errors"
	"fmt"
	"io"
	"net/rpc"
	"sync"

	"code.google.com/p/gogoprotobuf/proto"
	wire "github.com/cockroachdb/cockroach/rpc/codec/wire.pb"
)

type serverCodec struct {
	r io.Reader
	w io.Writer
	c io.Closer

	// temporary work space
	reqHeader wire.RequestHeader

	// Package rpc expects uint64 request IDs.
	// We assign uint64 sequence numbers to incoming requests
	// but save the original request ID in the pending map.
	// When rpc responds, we use the sequence number in
	// the response to find the original request ID.
	mutex   sync.Mutex // protects seq, pending
	seq     uint64
	pending map[uint64]uint64
}

// NewServerCodec returns a serverCodec that communicates with the ClientCodec
// on the other end of the given conn.
func NewServerCodec(conn io.ReadWriteCloser) rpc.ServerCodec {
	return &serverCodec{
		r:       conn,
		w:       conn,
		c:       conn,
		pending: make(map[uint64]uint64),
	}
}

func (c *serverCodec) ReadRequestHeader(r *rpc.Request) error {
	header := wire.RequestHeader{}
	err := readRequestHeader(c.r, &header)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	c.seq++
	c.pending[c.seq] = header.GetId()
	r.ServiceMethod = header.GetMethod()
	r.Seq = c.seq
	c.mutex.Unlock()

	c.reqHeader = header
	return nil
}

func (c *serverCodec) ReadRequestBody(x interface{}) error {
	if x == nil {
		return nil
	}
	request, ok := x.(proto.Message)
	if !ok {
		return fmt.Errorf(
			"protorpc.ServerCodec.ReadRequestBody: %T does not implement proto.Message",
			x,
		)
	}

	err := readRequestBody(c.r, &c.reqHeader, request)
	if err != nil {
		return nil
	}

	c.reqHeader = wire.RequestHeader{}
	return nil
}

// A value sent as a placeholder for the server's response value when the server
// receives an invalid request. It is never decoded by the client since the Response
// contains an error when it is used.
var invalidRequest = struct{}{}

func (c *serverCodec) WriteResponse(r *rpc.Response, x interface{}) error {
	var response proto.Message
	if x != nil {
		var ok bool
		if response, ok = x.(proto.Message); !ok {
			if _, ok = x.(struct{}); !ok {
				c.mutex.Lock()
				delete(c.pending, r.Seq)
				c.mutex.Unlock()
				return fmt.Errorf(
					"protorpc.ServerCodec.WriteResponse: %T does not implement proto.Message",
					x,
				)
			}
		}
	}

	c.mutex.Lock()
	id, ok := c.pending[r.Seq]
	if !ok {
		c.mutex.Unlock()
		return errors.New("protorpc: invalid sequence number in response")
	}
	delete(c.pending, r.Seq)
	c.mutex.Unlock()

	err := writeResponse(c.w, id, r.Error, response)
	if err != nil {
		return err
	}

	return nil
}

func (s *serverCodec) Close() error {
	return s.c.Close()
}

// ServeConn runs the Protobuf-RPC server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
// The caller typically invokes ServeConn in a go statement.
func ServeConn(conn io.ReadWriteCloser) {
	rpc.ServeCodec(NewServerCodec(conn))
}
