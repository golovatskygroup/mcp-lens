package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Transport handles MCP communication over stdio
type Transport struct {
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex
}

// NewTransport creates a new stdio transport
func NewTransport(r io.Reader, w io.Writer) *Transport {
	return &Transport{
		reader: bufio.NewReader(r),
		writer: w,
	}
}

// ReadMessage reads a JSON-RPC message from stdin
func (t *Transport) ReadMessage() (*Request, error) {
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &req, nil
}

// WriteResponse writes a JSON-RPC response to stdout
func (t *Transport) WriteResponse(resp *Response) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(t.writer, "%s\n", data)
	return err
}

// WriteNotification writes a JSON-RPC notification
func (t *Transport) WriteNotification(method string, params any) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var paramsData json.RawMessage
	if params != nil {
		var err error
		paramsData, err = json.Marshal(params)
		if err != nil {
			return err
		}
	}

	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsData,
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(t.writer, "%s\n", data)
	return err
}
