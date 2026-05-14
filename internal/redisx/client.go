package redisx

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	addr     string
	password string
	db       int
	useTLS   bool
	mu       sync.Mutex
	conn     net.Conn
	r        *bufio.Reader
}

func New(rawURL string) (*Client, error) {
	if rawURL == "" {
		rawURL = "redis://localhost:6379/0"
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return nil, fmt.Errorf("unsupported redis scheme %q", u.Scheme)
	}
	addr := u.Host
	if !strings.Contains(addr, ":") {
		addr += ":6379"
	}
	password, _ := u.User.Password()
	if password == "" && u.User != nil {
		password = u.User.Username()
	}
	db := 0
	if path := strings.Trim(u.Path, "/"); path != "" {
		db, _ = strconv.Atoi(path)
	}
	return &Client{addr: addr, password: password, db: db, useTLS: u.Scheme == "rediss"}, nil
}

func (c *Client) Do(ctx context.Context, args ...string) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConn(ctx); err != nil {
		return nil, err
	}
	if err := writeCommand(c.conn, args...); err != nil {
		c.closeLocked()
		return nil, err
	}
	v, err := readRESP(c.r)
	if err != nil {
		c.closeLocked()
	}
	return v, err
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) ensureConn(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	var conn net.Conn
	var err error
	if c.useTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", c.addr, &tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", c.addr)
	}
	if err != nil {
		return err
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	if c.password != "" {
		if err := writeCommand(conn, "AUTH", c.password); err != nil {
			c.closeLocked()
			return err
		}
		if _, err := readRESP(c.r); err != nil {
			c.closeLocked()
			return err
		}
	}
	if c.db > 0 {
		if err := writeCommand(conn, "SELECT", strconv.Itoa(c.db)); err != nil {
			c.closeLocked()
			return err
		}
		if _, err := readRESP(c.r); err != nil {
			c.closeLocked()
			return err
		}
	}
	return nil
}

func (c *Client) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.r = nil
	return err
}

func writeCommand(w io.Writer, args ...string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

func readRESP(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return nil, errors.New(line)
	case ':':
		return strconv.ParseInt(line, 10, 64)
	case '$':
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		arr := make([]any, n)
		for i := 0; i < n; i++ {
			v, err := readRESP(r)
			if err != nil {
				return nil, err
			}
			arr[i] = v
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unknown RESP prefix %q", prefix)
	}
}

func String(v any) string {
	s, _ := v.(string)
	return s
}

func Int(v any) int {
	switch t := v.(type) {
	case int64:
		return int(t)
	case string:
		i, _ := strconv.Atoi(t)
		return i
	default:
		return 0
	}
}
