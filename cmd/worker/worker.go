package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const version string = "0.1"

type config map[string]targetgroup

type targetgroup map[string]target

type target struct {
	Interval int
	Count    int
	Host     string
	Probe    string
}

// TODO(jamesog):
// Read shared secret from a file
// Connect to server over HTTP and fetch config (must be santised)
// Parse config, determine jobs to run and intervals
// Implement probes

func runProbe(tg *targetgroup) {
	for _, t := range *tg {
		probe := t.Probe
		interval := t.Interval
		count := t.Count
		host := t.Host
		go func() {
			log.Printf("Launching %d %s probes every %ds against %s\n",
				count, probe, interval, host)
			for {
				time.Sleep(time.Duration(interval) * time.Second)
				for n := 1; n <= count; n++ {
					log.Printf("PROBE %s (%d/%d): %s\n", probe, n, count, host)
				}
			}
		}()
	}
}

func main() {
	fmt.Println("NetSmog Worker, version", version)

	var server = flag.String("server", "", "server URL")
	var secretFile = flag.String("secret", "", "shared secret file")
	defaultWorker, err := os.Hostname()
	if err != nil {
		log.Fatal("could not determine hostname")
	}
	var worker = flag.String("worker", defaultWorker, "worker name")
	flag.Parse()
	if *server == "" {
		log.Fatal("no server specified")
	}
	if *secretFile == "" {
		log.Fatal("no shared secret file specified")
	}

	secret, err := ioutil.ReadFile(*secretFile)
	if err != nil {
		log.Fatal("could not read shared secret: ", err)
	}

	// Construct HTTP header for passing an authorisation to the server
	// This is a hash of worker:secret, similar to HTTP Basic
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(fmt.Sprintf("%s:%s", worker, secret)),
		bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("could not generate hash")
	}

	hashstr := base64.URLEncoding.EncodeToString(hash)
	fmt.Println("Hash:", hashstr)

	httpClient := &http.Client{}
	req, err := http.NewRequest("GET", *server, nil)
	if err != nil {
		log.Fatal("could not construct HTTP request")
	}
	req.Header.Add("User-Agent", fmt.Sprintf("NetSmog Worker version %s", version))
	req.Header.Add("Worker", *worker)
	req.Header.Add("Authorisation", hashstr)
	log.Println("fetching configuration from ", *server)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal("HTTP protocol error: ", err)
	}
	if resp.StatusCode == 200 {
		log.Println("got a response")
	} else {
		log.Fatal("something went wrong")
	}
	var config config
	jsonResponse, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("JSON read error")
	}
	err = json.Unmarshal(jsonResponse, &config)
	if err != nil {
		log.Fatal("config error")
	}
	log.Println("so far so good")
	// fmt.Printf("Config:\n%+v\n", config)
	c := make(chan struct{})
	for _, target := range config {
		runProbe(&target)
	}
	<-c
}
