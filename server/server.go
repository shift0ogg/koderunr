package main

import (
	"encoding/json"
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
}

func (s *Server) HandleRunCode(w http.ResponseWriter, r *http.Request) {
	uuid := r.FormValue("uuid")

	conn := s.redisPool.Get()
	defer conn.Close()

	value, err := redis.Bytes(conn.Do("GET", uuid+"#run"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot GET: %v\n", err)
		http.Error(w, "The source code doesn't exist", 422)
		return
	}

	// Started running code
	runner := &Runner{}
	json.Unmarshal(value, runner)

	isEvtStream := r.FormValue("evt") == "true"
	client := NewClient(runner, s.redisPool.Get(), uuid)

	go client.Write(w, isEvtStream)
	client.Run()

	// Purge the source code
	_, err = conn.Do("DEL", uuid+"#run")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to purge the source code for %s - %v\n", uuid, err)
	}
}

func (s *Server) HandleSaveCode(w http.ResponseWriter, r *http.Request) {
	runner := Runner{
		Ext:     r.FormValue("ext"),
		Source:  r.FormValue("source"),
		Version: r.FormValue("version"),
	}

	bts, _ := json.Marshal(&runner)
	strj := string(bts)

	codeID := NewRandID(10)

	conn := s.redisPool.Get()
	defer conn.Close()

	_, err := conn.Do("SET", codeID+"#snippet", strj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		http.Error(w, "A serious error has occured.", 500)
		return
	}

	fmt.Fprintf(w, codeID)
}

func (s *Server) HandleFetchCode(w http.ResponseWriter, r *http.Request) {
	codeID := r.FormValue("id")

	conn := s.redisPool.Get()
	defer conn.Close()

	value, err := redis.Bytes(conn.Do("GET", codeID+"#snippet"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot GET: %v\n", err)
		http.Error(w, "The source code doesn't exist", 422)
		return
	}

	w.Write(value)
}

func (s *Server) HandleReg(w http.ResponseWriter, r *http.Request) {
	runner := Runner{
		Ext:     r.FormValue("ext"),
		Source:  r.FormValue("source"),
		Version: r.FormValue("version"),
		Timeout: 15,
	}

	bts, _ := json.Marshal(&runner)
	strj := string(bts)

	cmd := exec.Command("uuidgen")
	output, _ := cmd.Output()
	uuid := strings.TrimSuffix(string(output), "\n")

	conn := s.redisPool.Get()
	defer conn.Close()

	_, err := conn.Do("SET", uuid+"#run", strj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		http.Error(w, "A serious error has occured.", 500)
		return
	}

	fmt.Fprint(w, uuid)
}

func (s *Server) HandleStdin(w http.ResponseWriter, r *http.Request) {
	input := r.FormValue("input")
	uuid := r.FormValue("uuid")

	conn := s.redisPool.Get()
	defer conn.Close()

	conn.Do("PUBLISH", uuid+"#stdin", input)

	fmt.Fprintf(w, "")
}

func (s *Server) HandleLangs(w http.ResponseWriter, r *http.Request) {
	text := `
Supported Languages:
  Ruby - 2.3.0
  Ruby - 1.9.3-p550
  Go - 1.6
  Elixir - 1.2.3
  Python - 2.7.6
  C
`
	text = strings.TrimSpace(text)

	fmt.Fprintf(w, "%s\n", text)
}
