// Package kuromi implements a framework for dealing with WebSockets.
//
// Example
//
// A broadcasting echo server:
//
//  func main() {
//  	m := kurom.New()
//  	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
//  		m.HandleRequest(w, r)
//  	})
//  	m.HandleMessage(func(s *kuromi.Session, msg []byte) {
//  		m.Broadcast(msg)
//  	})
//  	http.ListenAndServe(":5000", nil)
//  }

package kuromi
