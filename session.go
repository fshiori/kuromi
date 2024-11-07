package kuromi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Session wrapper around websocket connections.
type Session struct {
	Request    *http.Request
	Keys       map[string]any
	conn       *websocket.Conn
	output     chan envelope
	outputDone chan struct{}
	kuromi     *Kuromi
	open       bool
	rwmutex    *sync.RWMutex
}

func (s *Session) writeMessage(message envelope) {
	if s.closed() {
		s.kuromi.errorHandler(s, ErrWriteClosed)
		return
	}

	select {
	case s.output <- message:
	default:
		s.kuromi.errorHandler(s, ErrMessageBufferFull)
	}
}

func (s *Session) writeRaw(message envelope) error {
	if s.closed() {
		return ErrWriteClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.kuromi.Config.WriteWait)
	defer cancel()
	err := s.conn.Write(ctx, message.t, message.msg)

	if err != nil {
		return err
	}

	return nil
}

func (s *Session) closed() bool {
	s.rwmutex.RLock()
	defer s.rwmutex.RUnlock()

	return !s.open
}

func (s *Session) close() {
	s.closeWithMsg(websocket.StatusNormalClosure, "")
}

func (s *Session) closeWithMsg(code websocket.StatusCode, reason string) {
	s.rwmutex.Lock()
	open := s.open
	s.open = false
	s.rwmutex.Unlock()
	if open {
		s.conn.Close(code, reason)
		close(s.outputDone)
		if s.kuromi.closeHandler != nil {
			s.kuromi.closeHandler(s, int(code), reason)
		}
	}
}

func (s *Session) ping() {
	ctx, cancel := context.WithTimeout(context.Background(), s.kuromi.Config.WriteWait)
	defer cancel()
	err := s.conn.Ping(ctx)
	if err != nil && s.kuromi.pongHandler != nil {
		s.kuromi.pongHandler(s)
	}
}

func (s *Session) writePump() {
	ticker := time.NewTicker(s.kuromi.Config.PingPeriod)
	defer ticker.Stop()

loop:
	for {
		select {
		case msg := <-s.output:
			if msg.t == CloseMessage {
				s.closeWithMsg(msg.code, string(msg.msg))
				return
			}

			err := s.writeRaw(msg)

			if err != nil {
				s.kuromi.errorHandler(s, err)
				break loop
			}

			if msg.t == websocket.MessageText {
				s.kuromi.messageSentHandler(s, msg.msg)
			}

			if msg.t == websocket.MessageBinary {
				s.kuromi.messageSentHandlerBinary(s, msg.msg)
			}
		case <-ticker.C:
			s.ping()
		case _, ok := <-s.outputDone:
			if !ok {
				break loop
			}
		}
	}

	s.close()
}

func (s *Session) readPump() {
	s.conn.SetReadLimit(s.kuromi.Config.MaxMessageSize)

	for {
		// TODO: add timeout ref: readdeadline
		t, message, err := s.conn.Read(context.Background())

		if err != nil {
			s.kuromi.errorHandler(s, err)
			break
		}

		if s.kuromi.Config.ConcurrentMessageHandling {
			go s.handleMessage(t, message)
		} else {
			s.handleMessage(t, message)
		}
	}
}

func (s *Session) handleMessage(t websocket.MessageType, message []byte) {
	switch t {
	case websocket.MessageText:
		s.kuromi.messageHandler(s, message)
	case websocket.MessageBinary:
		s.kuromi.messageHandlerBinary(s, message)
	}
}

// Write writes message to session.
func (s *Session) Write(msg []byte) error {
	if s.closed() {
		return ErrSessionClosed
	}

	s.writeMessage(envelope{t: websocket.MessageText, msg: msg})

	return nil
}

// WriteBinary writes a binary message to session.
func (s *Session) WriteBinary(msg []byte) error {
	if s.closed() {
		return ErrSessionClosed
	}

	s.writeMessage(envelope{t: websocket.MessageBinary, msg: msg})

	return nil
}

// Close closes session.
func (s *Session) Close() error {
	if s.closed() {
		return ErrSessionClosed
	}

	s.writeMessage(envelope{t: CloseMessage, msg: []byte{}, code: websocket.StatusNormalClosure})

	return nil
}

// CloseWithMsg closes the session with the provided payload.
// Use the FormatCloseMessage function to format a proper close message payload.
func (s *Session) CloseWithMsg(code websocket.StatusCode, reason string) error {
	if s.closed() {
		return ErrSessionClosed
	}

	s.writeMessage(envelope{t: CloseMessage, msg: []byte(reason), code: code})

	return nil
}

// Set is used to store a new key/value pair exclusively for this session.
// It also lazy initializes s.Keys if it was not used previously.
func (s *Session) Set(key string, value any) {
	s.rwmutex.Lock()
	defer s.rwmutex.Unlock()

	if s.Keys == nil {
		s.Keys = make(map[string]any)
	}

	s.Keys[key] = value
}

// Get returns the value for the given key, ie: (value, true).
// If the value does not exists it returns (nil, false)
func (s *Session) Get(key string) (value any, exists bool) {
	s.rwmutex.RLock()
	defer s.rwmutex.RUnlock()

	if s.Keys != nil {
		value, exists = s.Keys[key]
	}

	return
}

// MustGet returns the value for the given key if it exists, otherwise it panics.
func (s *Session) MustGet(key string) any {
	if value, exists := s.Get(key); exists {
		return value
	}

	panic("Key \"" + key + "\" does not exist")
}

// UnSet will delete the key and has no return value
func (s *Session) UnSet(key string) {
	s.rwmutex.Lock()
	defer s.rwmutex.Unlock()
	if s.Keys != nil {
		delete(s.Keys, key)
	}
}

// IsClosed returns the status of the connection.
func (s *Session) IsClosed() bool {
	return s.closed()
}

// WebsocketConnection returns the underlying websocket connection.
// This can be used to e.g. set/read additional websocket options or to write sychronous messages.
func (s *Session) WebsocketConnection() *websocket.Conn {
	return s.conn
}
