# kuromi

<img align="right" width="159px" src="https://raw.githubusercontent.com/fshiori/kuromi/main/kuromies_icon04.png">

> Minimalist websocket framework for Go.
> 世界クロミ化計画

Kuromi is websocket framework based on [github.com/coder/websocket](github.com/coder/websocket)
and rewrite of [github.com/olahol/melody](github.com/olahol/melody)
that abstracts away the tedious parts of handling websockets. It gets out of
your way so you can write real-time apps. Features include:

* [x] Clear and easy interface similar to `net/http` or Gin.
* [x] A simple way to broadcast to all or selected connected sessions.
* [x] Message buffers making concurrent writing safe.
* [x] Automatic handling of sending ping/pong heartbeats that timeout broken sessions.
* [x] Store data on sessions.

## Install

```bash
go get github.com/fshiori/kuromi
```

## [Example: chat](https://github.com/fshiori/kuromi/tree/main/examples/chat)

[![Chat](https://raw.githubusercontent.com/fshiori/kuromi/main/examples/chat/demo.gif "Demo")](https://github.com/fshiori/kuromi/tree/main/examples/chat)

```go
package main

import (
	"net/http"

	"github.com/fshiori/kuromi"
)

func main() {
	k := kuromi.New()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		k.HandleRequest(w, r)
	})

	k.HandleMessage(func(s *kuromi.Session, msg []byte) {
		k.Broadcast(msg)
	})

	http.ListenAndServe(":5000", nil)
}
```