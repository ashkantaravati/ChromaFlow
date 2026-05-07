package realtime

import (
	"bufio"
	"chromaflow/internal/queue"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

type client struct {
	conn net.Conn
	send chan []byte
}

type JobsMessage struct {
	Type string              `json:"type"`
	Jobs []queue.JobSnapshot `json:"jobs"`
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*client]struct{}),
	}
}

func (h *Hub) BroadcastJobs(jobs []queue.JobSnapshot) {
	message, err := json.Marshal(JobsMessage{Type: "jobs", Jobs: jobs})
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- message:
		default:
		}
	}
}

func (h *Hub) ServeJobs(w http.ResponseWriter, r *http.Request, snapshot func() []queue.JobSnapshot) {
	conn, rw, err := accept(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c := &client{
		conn: conn,
		send: make(chan []byte, 8),
	}
	h.add(c)
	defer h.remove(c)
	defer conn.Close()

	initial, err := json.Marshal(JobsMessage{Type: "jobs", Jobs: snapshot()})
	if err == nil {
		c.send <- initial
	}

	done := make(chan struct{})
	go waitForClose(conn, done)

	for {
		select {
		case message := <-c.send:
			if err := writeTextFrame(rw, message); err != nil {
				return
			}
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
}

func accept(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, nil, errors.New("missing websocket upgrade header")
	}
	if !headerContains(r.Header.Get("Connection"), "upgrade") {
		return nil, nil, errors.New("missing websocket connection header")
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, nil, errors.New("missing websocket key")
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("websocket hijacking is not supported")
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}

	acceptKey := websocketAcceptKey(key)
	_, err = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", acceptKey)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}

	return conn, rw, nil
}

func websocketAcceptKey(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerContains(header string, value string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), value) {
			return true
		}
	}
	return false
}

func writeTextFrame(rw *bufio.ReadWriter, payload []byte) error {
	header := []byte{0x81}
	length := len(payload)

	switch {
	case length <= 125:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[2:], uint16(length))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[2:], uint64(length))
	}

	if _, err := rw.Write(header); err != nil {
		return err
	}
	if _, err := rw.Write(payload); err != nil {
		return err
	}
	return rw.Flush()
}

func waitForClose(conn net.Conn, done chan<- struct{}) {
	defer close(done)
	_, _ = io.Copy(io.Discard, conn)
}
