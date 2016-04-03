package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/garyburd/redigo/redis"
)

// Server is the abstraction of a koderunr web api
type Server struct {
	redisPool *redis.Pool
	broker    *Broker
}

func (s *Server) handleRunCode(w http.ResponseWriter, r *http.Request) {
	uuid := r.FormValue("uuid")

	conn := s.redisPool.Get()
	defer conn.Close()

	value, err := redis.Bytes(conn.Do("GET", uuid))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot GET: %v\n", err)
		http.Error(w, "The source code doesn't exist", 422)
		return
	}

	// Started running code
	runner := &Runner{}
	json.Unmarshal(value, runner)

	isEvtStream := r.FormValue("evt") == "true"
	client := NewClient(runner)

	go client.Write(w, isEvtStream)
	client.Run()

	// Purge the source code
	_, err = conn.Do("DEL", uuid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to purge the source code for %s - %v", uuid, err)
	}
}

func (s *Server) handleReg(w http.ResponseWriter, r *http.Request) {
	runner := Runner{
		r.FormValue("ext"),
		r.FormValue("source"),
		r.FormValue("version"),
	}

	bts, _ := json.Marshal(&runner)
	strj := string(bts)

	cmd := exec.Command("uuidgen")
	output, _ := cmd.Output()
	uuid := strings.TrimSuffix(string(output), "\n")

	conn := s.redisPool.Get()
	defer conn.Close()

	_, err := conn.Do("SET", uuid, strj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		http.Error(w, "A serious error has occured.", 500)
		return
	}

	fmt.Fprint(w, uuid)
}

var servingStatic bool

func init() {
	flag.BoolVar(&servingStatic, "static", false, "if using Go server hosting static files")
	flag.Parse()
}

func main() {
	redisPool := redis.NewPool(func() (redis.Conn, error) {
		conn, err := redis.Dial("tcp", ":6379")
		if err != nil {
			return nil, err
		}
		return conn, err
	}, 4)

	s := &Server{
		redisPool: redisPool,
		broker:    NewBroker(),
	}

	go s.broker.Start()

	if servingStatic {
		http.Handle("/", http.FileServer(http.Dir("static")))
	}

	http.HandleFunc("/run", s.handleRunCode)
	http.HandleFunc("/register/", s.handleReg)
	http.ListenAndServe(":8080", nil)
}
