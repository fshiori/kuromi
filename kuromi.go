package kuromi

import (
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

const (
	CloseMessage websocket.MessageType = websocket.MessageText + 1000
)

type handleMessageFunc func(*Session, []byte)
type handleErrorFunc func(*Session, error)
type handleCloseFunc func(*Session, int, string) error
type handleSessionFunc func(*Session)
type filterFunc func(*Session) bool

// Kuromi implements a websocket manager.
type Kuromi struct {
	Config                   *Config
	AcceptOptions            *websocket.AcceptOptions
	messageHandler           handleMessageFunc
	messageHandlerBinary     handleMessageFunc
	messageSentHandler       handleMessageFunc
	messageSentHandlerBinary handleMessageFunc
	errorHandler             handleErrorFunc
	closeHandler             handleCloseFunc
	connectHandler           handleSessionFunc
	disconnectHandler        handleSessionFunc
	pongHandler              handleSessionFunc
	hub                      *hub
}

// New creates a new kuromi instance with default Upgrader and Config.
func New() *Kuromi {
	hub := newHub()

	go hub.run()

	return &Kuromi{
		Config:                   newConfig(),
		AcceptOptions:            nil,
		messageHandler:           func(*Session, []byte) {},
		messageHandlerBinary:     func(*Session, []byte) {},
		messageSentHandler:       func(*Session, []byte) {},
		messageSentHandlerBinary: func(*Session, []byte) {},
		errorHandler:             func(*Session, error) {},
		closeHandler:             nil,
		connectHandler:           func(*Session) {},
		disconnectHandler:        func(*Session) {},
		pongHandler:              func(*Session) {},
		hub:                      hub,
	}
}

// HandleConnect fires fn when a session connects.
func (k *Kuromi) HandleConnect(fn func(*Session)) {
	k.connectHandler = fn
}

// HandleDisconnect fires fn when a session disconnects.
func (k *Kuromi) HandleDisconnect(fn func(*Session)) {
	k.disconnectHandler = fn
}

// HandlePong fires fn when a pong is received from a session.
func (k *Kuromi) HandlePong(fn func(*Session)) {
	k.pongHandler = fn
}

// HandleMessage fires fn when a text message comes in.
// NOTE: by default Kuromi handles messages sequentially for each
// session. This has the effect that a message handler exceeding the
// read deadline (Config.PongWait, by default 1 minute) will time out
// the session. Concurrent message handling can be turned on by setting
// Config.ConcurrentMessageHandling to true.
func (k *Kuromi) HandleMessage(fn func(*Session, []byte)) {
	k.messageHandler = fn
}

// HandleMessageBinary fires fn when a binary message comes in.
func (k *Kuromi) HandleMessageBinary(fn func(*Session, []byte)) {
	k.messageHandlerBinary = fn
}

// HandleSentMessage fires fn when a text message is successfully sent.
func (k *Kuromi) HandleSentMessage(fn func(*Session, []byte)) {
	k.messageSentHandler = fn
}

// HandleSentMessageBinary fires fn when a binary message is successfully sent.
func (k *Kuromi) HandleSentMessageBinary(fn func(*Session, []byte)) {
	k.messageSentHandlerBinary = fn
}

// HandleError fires fn when a session has an error.
func (k *Kuromi) HandleError(fn func(*Session, error)) {
	k.errorHandler = fn
}

// HandleClose sets the handler for close messages received from the session.
// The code argument to h is the received close code or CloseNoStatusReceived
// if the close message is empty. The default close handler sends a close frame
// back to the session.
//
// The application must read the connection to process close messages as
// described in the section on Control Frames above.
//
// The connection read methods return a CloseError when a close frame is
// received. Most applications should handle close messages as part of their
// normal error handling. Applications should only set a close handler when the
// application must perform some action before sending a close frame back to
// the session.
func (k *Kuromi) HandleClose(fn func(*Session, int, string) error) {
	if fn != nil {
		k.closeHandler = fn
	}
}

// HandleRequest upgrades http requests to websocket connections and dispatches them to be handled by the kuromi instance.
func (k *Kuromi) HandleRequest(w http.ResponseWriter, r *http.Request) error {
	return k.HandleRequestWithKeys(w, r, nil)
}

// HandleRequestWithKeys does the same as HandleRequest but populates session.Keys with keys.
func (k *Kuromi) HandleRequestWithKeys(w http.ResponseWriter, r *http.Request, keys map[string]any) error {
	if k.hub.closed() {
		return ErrClosed
	}

	c, err := websocket.Accept(w, r, k.AcceptOptions)

	if err != nil {
		return err
	}

	session := &Session{
		Request:    r,
		Keys:       keys,
		conn:       c,
		output:     make(chan envelope, k.Config.MessageBufferSize),
		outputDone: make(chan struct{}),
		kuromi:     k,
		open:       true,
		rwmutex:    &sync.RWMutex{},
	}

	k.hub.register <- session

	k.connectHandler(session)

	go session.writePump()

	session.readPump()

	if !k.hub.closed() {
		k.hub.unregister <- session
	}

	session.close()

	k.disconnectHandler(session)

	return nil
}

// Broadcast broadcasts a text message to all sessions.
func (k *Kuromi) Broadcast(msg []byte) error {
	if k.hub.closed() {
		return ErrClosed
	}

	message := envelope{t: websocket.MessageText, msg: msg}
	k.hub.broadcast <- message

	return nil
}

// BroadcastFilter broadcasts a text message to all sessions that fn returns true for.
func (k *Kuromi) BroadcastFilter(msg []byte, fn func(*Session) bool) error {
	if k.hub.closed() {
		return ErrClosed
	}

	message := envelope{t: websocket.MessageText, msg: msg, filter: fn}
	k.hub.broadcast <- message

	return nil
}

// BroadcastOthers broadcasts a text message to all sessions except session s.
func (k *Kuromi) BroadcastOthers(msg []byte, s *Session) error {
	return k.BroadcastFilter(msg, func(q *Session) bool {
		return s != q
	})
}

// BroadcastMultiple broadcasts a text message to multiple sessions given in the sessions slice.
func (k *Kuromi) BroadcastMultiple(msg []byte, sessions []*Session) error {
	for _, sess := range sessions {
		if writeErr := sess.Write(msg); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// BroadcastBinary broadcasts a binary message to all sessions.
func (k *Kuromi) BroadcastBinary(msg []byte) error {
	if k.hub.closed() {
		return ErrClosed
	}

	message := envelope{t: websocket.MessageBinary, msg: msg}
	k.hub.broadcast <- message

	return nil
}

// BroadcastBinaryFilter broadcasts a binary message to all sessions that fn returns true for.
func (k *Kuromi) BroadcastBinaryFilter(msg []byte, fn func(*Session) bool) error {
	if k.hub.closed() {
		return ErrClosed
	}

	message := envelope{t: websocket.MessageBinary, msg: msg, filter: fn}
	k.hub.broadcast <- message

	return nil
}

// BroadcastBinaryOthers broadcasts a binary message to all sessions except session s.
func (k *Kuromi) BroadcastBinaryOthers(msg []byte, s *Session) error {
	return k.BroadcastBinaryFilter(msg, func(q *Session) bool {
		return s != q
	})
}

// Sessions returns all sessions. An error is returned if the kuromi session is closed.
func (k *Kuromi) Sessions() ([]*Session, error) {
	if k.hub.closed() {
		return nil, ErrClosed
	}
	return k.hub.all(), nil
}

// Close closes the kuromi instance and all connected sessions.
func (k *Kuromi) Close() error {
	if k.hub.closed() {
		return ErrClosed
	}

	k.hub.exit <- envelope{t: CloseMessage, msg: []byte{}, code: websocket.StatusNormalClosure}

	return nil
}

// CloseWithMsg closes the kuromi instance with the given close payload and all connected sessions.
// Use the FormatCloseMessage function to format a proper close message payload.
func (k *Kuromi) CloseWithMsg(code websocket.StatusCode, reason string) error {
	if k.hub.closed() {
		return ErrClosed
	}

	k.hub.exit <- envelope{t: CloseMessage, msg: []byte(reason), code: code}

	return nil
}

// TODO: CloseNow

// Len return the number of connected sessions.
func (k *Kuromi) Len() int {
	return k.hub.len()
}

// IsClosed returns the status of the kuromi instance.
func (k *Kuromi) IsClosed() bool {
	return k.hub.closed()
}
