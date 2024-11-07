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
