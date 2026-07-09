// Command simserver runs the in-memory workspace simulator as a standalone
// HTTP server, so the real `orq` binary can be exercised end-to-end with zero
// production traffic:
//
//	go run ./cli/custom/dsl/simserver -port 7899 &
//	ORQ_SERVER=http://localhost:7899 ORQ_API_KEY=dry orq dsl plan -f ./stack
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"orq/cli/custom/dsl"
)

func main() {
	port := flag.Int("port", 7899, "listen port")
	flag.Parse()
	sim := dsl.NewSimulator()
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("workspace simulator listening on http://%s (in-memory, throwaway)", addr)
	log.Fatal(http.ListenAndServe(addr, sim.Handler()))
}
