package kuromi

import "github.com/coder/websocket"

type envelope struct {
	t      websocket.MessageType
	msg    []byte
	filter filterFunc

	code websocket.StatusCode // only used for close message
}
