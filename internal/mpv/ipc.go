package mpv

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Client talks mpv's JSON IPC protocol over a unix socket or Windows named pipe.
// It is used to hand files to the background player and to drive OSD messages
// while a cloud file hydrates or an R3D proxy renders.
type Client struct {
	conn    io.ReadWriteCloser
	r       *bufio.Reader
	mu      sync.Mutex
	nextID  int
	timeout time.Duration
}

type response struct {
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	RequestID int             `json:"request_id"`
	Event     string          `json:"event"`
}

// Dial connects to the IPC endpoint, retrying briefly (mpv creates the socket a
// moment after it starts).
func Dial(path string, wait time.Duration) (*Client, error) {
	deadline := time.Now().Add(wait)
	var lastErr error
	for {
		conn, err := dial(path)
		if err == nil {
			return &Client{conn: conn, r: bufio.NewReaderSize(conn, 64<<10), timeout: 5 * time.Second}, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Command sends a command and returns the "data" field of the reply.
func (c *Client) Command(args ...any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	id := c.nextID
	req, err := json.Marshal(map[string]any{"command": args, "request_id": id})
	if err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("mpv ipc write: %w", err)
	}
	type result struct {
		resp response
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			line, err := c.r.ReadBytes('\n')
			if err != nil {
				ch <- result{err: fmt.Errorf("mpv ipc read: %w", err)}
				return
			}
			var resp response
			if json.Unmarshal(line, &resp) != nil {
				continue
			}
			if resp.Event != "" { // asynchronous event, not ours
				continue
			}
			if resp.RequestID == id || resp.RequestID == 0 {
				ch <- result{resp: resp}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		if r.resp.Error != "success" {
			return nil, fmt.Errorf("mpv: %s", r.resp.Error)
		}
		return r.resp.Data, nil
	case <-time.After(c.timeout):
		c.conn.Close()
		return nil, errors.New("mpv ipc: timeout waiting for reply")
	}
}

// SetProperty sets a property.
func (c *Client) SetProperty(name string, value any) error {
	_, err := c.Command("set_property", name, value)
	return err
}

// GetProperty reads a property into v.
func (c *Client) GetProperty(name string, v any) error {
	data, err := c.Command("get_property", name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// LoadFile loads a file. mode is "replace", "append" or "append-play".
// options are per-file options such as "video-rotate=90".
func (c *Client) LoadFile(path, mode string, options ...string) error {
	args := []any{"loadfile", path, mode}
	if len(options) > 0 {
		args = append(args, joinOpts(options))
	}
	_, err := c.Command(args...)
	return err
}

func joinOpts(opts []string) string {
	s := ""
	for i, o := range opts {
		if i > 0 {
			s += ","
		}
		s += o
	}
	return s
}

// ShowText shows an OSD message for durationMs milliseconds.
func (c *Client) ShowText(text string, durationMs int) error {
	_, err := c.Command("show-text", text, durationMs)
	return err
}

// Raise tries to bring the player window to the front.
func (c *Client) Raise() {
	_ = c.SetProperty("window-minimized", false)
	var ontop bool
	_ = c.GetProperty("ontop", &ontop)
	if !ontop {
		_ = c.SetProperty("ontop", true)
		time.Sleep(150 * time.Millisecond)
		_ = c.SetProperty("ontop", false)
	}
}

// Ping checks that mpv answers.
func (c *Client) Ping() bool {
	var pid int
	return c.GetProperty("pid", &pid) == nil
}
