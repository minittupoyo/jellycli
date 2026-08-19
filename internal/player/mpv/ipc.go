package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
)

type ipcRequest struct {
	Command   []any  `json:"command"`
	RequestID uint64 `json:"request_id"`
}
type ipcMessage struct {
	RequestID uint64          `json:"request_id"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	Event     string          `json:"event"`
	Reason    string          `json:"reason"`
	FileError string          `json:"file_error"`
}

type ipcClient struct {
	conn      net.Conn
	writeMu   sync.Mutex
	mu        sync.Mutex
	nextID    uint64
	pending   map[uint64]chan ipcMessage
	events    chan ipcMessage
	done      chan struct{}
	closeOnce sync.Once
	err       error
}

func newIPCClient(conn net.Conn) *ipcClient {
	c := &ipcClient{conn: conn, pending: make(map[uint64]chan ipcMessage), events: make(chan ipcMessage, 32), done: make(chan struct{})}
	go c.readLoop()
	return c
}
func (c *ipcClient) Events() <-chan ipcMessage { return c.events }

func (c *ipcClient) command(ctx context.Context, destination any, name string, args ...any) error {
	command := make([]any, 1, len(args)+1)
	command[0] = name
	command = append(command, args...)
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := c.nextID
	reply := make(chan ipcMessage, 1)
	c.pending[id] = reply
	c.mu.Unlock()

	payload, err := json.Marshal(ipcRequest{Command: command, RequestID: id})
	if err == nil {
		c.writeMu.Lock()
		_, err = c.conn.Write(append(payload, '\n'))
		c.writeMu.Unlock()
	}
	if err != nil {
		c.removePending(id)
		return fmt.Errorf("%w: write command: %v", ErrIPC, err)
	}
	select {
	case message := <-reply:
		return decodeReply(message, destination, name)
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.done:
		// mpv can send the successful quit reply immediately before closing the
		// socket. Prefer that already-buffered reply over the close notification.
		select {
		case message := <-reply:
			return decodeReply(message, destination, name)
		default:
		}
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		return err
	}
}

func decodeReply(message ipcMessage, destination any, name string) error {
	if message.Error != "success" {
		return fmt.Errorf("%w: command %s: %s", ErrIPC, name, message.Error)
	}
	if destination != nil && len(message.Data) > 0 {
		if err := json.Unmarshal(message.Data, destination); err != nil {
			return fmt.Errorf("%w: decode command %s: %v", ErrIPC, name, err)
		}
	}
	return nil
}
func (c *ipcClient) removePending(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}
func (c *ipcClient) readLoop() {
	decoder := json.NewDecoder(bufio.NewReader(c.conn))
	for {
		var message ipcMessage
		if err := decoder.Decode(&message); err != nil {
			c.fail(err)
			return
		}
		if message.RequestID != 0 {
			c.mu.Lock()
			reply := c.pending[message.RequestID]
			delete(c.pending, message.RequestID)
			c.mu.Unlock()
			if reply != nil {
				reply <- message
			}
		} else if message.Event != "" {
			select {
			case c.events <- message:
			default:
			}
		}
	}
}
func (c *ipcClient) fail(cause error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.err = fmt.Errorf("%w: connection closed: %v", ErrIPC, cause)
		c.pending = make(map[uint64]chan ipcMessage)
		c.mu.Unlock()
		close(c.done)
		close(c.events)
		_ = c.conn.Close()
	})
}
func (c *ipcClient) Close() { c.fail(io.EOF) }
