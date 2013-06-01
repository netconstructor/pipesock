package main

import (
	"bufio"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"flag"
	"fmt"
	"go/build"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Hub struct {
	Connections map[*Socket]bool
	Pipe        chan string
}

type Message struct {
	Time    time.Time
	Message string
}

type Broadcast struct {
	Time     time.Time
	Messages []*Message
}

func (h *Hub) BroadcastLoop() {
	var currentMessages []*Message
	for {
		select {

		// Pipe in
		case str := <-h.Pipe:
			newMessage := &Message{time.Now(), str}
			currentMessages = append(currentMessages, newMessage)

			//Broadcast
		case <-time.After(time.Duration(delayMillis) * time.Millisecond):
			if len(currentMessages) > 0 {
				broadcast := &Broadcast{time.Now(), currentMessages}
				broadcastJSON, err := json.Marshal(broadcast)

				if err != nil {
					log.Println("Buffer JSON Error: ", err)
					return
				}

				for s, _ := range h.Connections {
					err := websocket.Message.Send(s.Ws, string(broadcastJSON))
					if err != nil {
						log.Println("WS error:", err)
						s.Ws.Close()
						delete(h.Connections, s)
					}
				}
				// Push onto buffer, or grow if not yet at max
				if len(broadcastBuffer) == bufferSize {
					for i := 1; i < bufferSize-1; i++ {
						broadcastBuffer[i-1] = broadcastBuffer[i]
					}
					broadcastBuffer[bufferSize-1] = broadcast
				} else {
					broadcastBuffer = append(broadcastBuffer, broadcast)
				}
				currentMessages = currentMessages[:0]
			}
		}
	}
}

type Socket struct {
	Ws *websocket.Conn
}

func (s *Socket) ReceiveMessage() {

	for {
		var x []byte
		err := websocket.Message.Receive(s.Ws, &x)
		if err != nil {
			break
		}
	}
	s.Ws.Close()
}

func readLoop() {
	r := bufio.NewReader(os.Stdin)
	for {
		str, err := r.ReadString('\n')
		if err != nil {
			log.Println("Read Line Error:", err)
			continue
		}
		if len(str) == 0 {
			continue
		}
		if passThrough {
			fmt.Print(str)
		}
		h.Pipe <- str
	}
}

func IndexHandler(w http.ResponseWriter, req *http.Request) {

	var filePath string

	if req.URL.Path == "/" {
		filePath = fmt.Sprintf("%s/index.html", viewPath)
	} else {
		filePath = fmt.Sprintf("%s/%s", viewPath, req.URL.Path)
	}
	if logging {
		log.Println(filePath)
	}
	http.ServeFile(w, req, filePath)
}

func BufferHandler(w http.ResponseWriter, req *http.Request) {
	broadcastBufferJSON, err := json.Marshal(broadcastBuffer)
	if err != nil {
		log.Println("Buffer JSON Error: ", err)
		return
	}
	fmt.Fprintf(w, string(broadcastBufferJSON))
}

func FlushHandler(w http.ResponseWriter, req *http.Request) {
	broadcastBuffer = broadcastBuffer[:0]
	fmt.Fprintf(w, "Flushed")
}

func wsServer(ws *websocket.Conn) {
	s := &Socket{ws}
	h.Connections[s] = true
	s.ReceiveMessage()
}

var (
	h                             Hub
	viewPath                      string
	port, bufferSize, delayMillis int
	passThrough, logging          bool
	broadcastBuffer               []*Broadcast
)

func init() {
	flag.IntVar(&port, "port", 9193, "Port for the pipesock to sit on.")
	flag.IntVar(&port, "p", 9193, "Port for the pipesock to sit on (shorthand).")

	flag.BoolVar(&passThrough, "through", false, "Pass output to STDOUT.")
	flag.BoolVar(&passThrough, "t", false, "Pass output to STDOUT (shorthand).")

	flag.BoolVar(&logging, "log", false, "Log HTTP requetsts to STDOUT")
	flag.BoolVar(&logging, "l", false, "Log HTTP requests tp STDOUT (shorthand).")

	flag.StringVar(&viewPath, "view", "default", "View directory to serve.")
	flag.StringVar(&viewPath, "v", "default", "View directory to serve (shorthand).")

	flag.IntVar(&bufferSize, "num", 20, "Number of previous broadcasts to keep in memory.")
	flag.IntVar(&bufferSize, "n", 20, "Number of previous broadcasts to keep in memory (shorthand).")

	flag.IntVar(&delayMillis, "delay", 2000, "Delay between broadcasts of bundled events in ms.")
	flag.IntVar(&delayMillis, "d", 2000, "Delay between broadcasts of bundled events in ms (shorthand).")

	broadcastBuffer = make([]*Broadcast, 0)

	// Set up hub
	h.Connections = make(map[*Socket]bool)
	h.Pipe = make(chan string, 1)
}

func main() {
	flag.Parse()

	pkgInfo, err := build.Import("github.com/minikomi/pipesock", "", 0)
	if err != nil {
		panic(err)
	}
	viewPath = filepath.Join(pkgInfo.Dir, "views", viewPath)
	fmt.Println(viewPath)

	go h.BroadcastLoop()
	go readLoop()

	http.HandleFunc("/", IndexHandler)
	http.HandleFunc("/buffer.json", BufferHandler)
	http.HandleFunc("/flush", FlushHandler)
	http.Handle("/ws", websocket.Handler(wsServer))

	portString := fmt.Sprintf(":%d", port)
	err = http.ListenAndServe(portString, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
