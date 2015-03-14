package main

import (
	"flag"
	"fmt"
	"log"
)

const VERSION string = "0.1"

// TODO(jamesog):
// Read shared secret from a file
// Connect to server over HTTP and fetch config (must be santised)
// Parse config, determine jobs to run and intervals
// Implement probes

func main() {
	fmt.Println("NetSmog Worker, version", VERSION)

	var server = flag.String("server", "", "server URL")
	flag.Parse()
	if *server == "" {
		log.Fatal("no server specified")
	}
}
