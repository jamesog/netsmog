package main

import (
	"bytes"
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

var (
	server     string
	secretFile string
	worker     string
)

// TODO(jamesog):
// Read shared secret from a file
// Connect to server over HTTP and fetch config (must be santised)
// Parse config, determine jobs to run and intervals
// Implement probes

func runProbe(g, t string, target *target) {
	probe := target.Probe
	interval := target.Interval
	count := target.Count
	host := target.Host
	go func() {
		log.Printf("Launching %d %s probes every %ds against %s\n",
			count, probe, interval, host)
		for {
			time.Sleep(time.Duration(interval) * time.Second)
			for n := 1; n <= count; n++ {
				log.Printf("PROBE %s (%d/%d): %s\n", probe, n, count, host)
				// TODO(jamesog): Implement probe
			}
			// TODO(jamesog): Submit results to server
			log.Printf("Submitting results for %s.%s\n", g, t)
			// TODO(jamesog): If submit fails, cache it and retry later
			// Perhaps all submits should be cached anyway
		}
	}()
}

func makeAuthorisation() (string, error) {
	secret, err := ioutil.ReadFile(secretFile)
	if err != nil {
		log.Fatal("could not read shared secret: ", err)
	}

	hash, err := bcrypt.GenerateFromPassword(
		[]byte(fmt.Sprintf("%s:%s", worker, secret)),
		bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	hashstr := base64.URLEncoding.EncodeToString(hash)
	return hashstr, nil
}

func httpRequest(method, server string, body []byte) (*http.Response, error) {
	httpClient := &http.Client{}
	req, err := http.NewRequest(method, server, bytes.NewBuffer(body))
	if err != nil {
		log.Fatal("could not construct HTTP request")
	}
	// Construct HTTP header for passing an authorisation to the server
	// This is a hash of worker:secret, similar to HTTP Basic
	auth, err := makeAuthorisation()
	if err != nil {
		log.Fatal("could not generate authorisation")
	}
	req.Header.Add("User-Agent", fmt.Sprintf("NetSmog Worker version %s", version))
	req.Header.Add("Worker", worker)
	req.Header.Add("Authorisation", auth)
	resp, err := httpClient.Do(req)
	return resp, err
}

func main() {
	fmt.Println("NetSmog Worker, version", version)

	flag.StringVar(&server, "server", "", "server URL")
	flag.StringVar(&secretFile, "secret", "", "shared secret file")
	defaultWorker, err := os.Hostname()
	if err != nil {
		log.Fatal("could not determine hostname")
	}
	flag.StringVar(&worker, "worker", defaultWorker, "worker name")
	flag.Parse()
	if server == "" {
		log.Fatal("no server specified")
	}
	if secretFile == "" {
		log.Fatal("no shared secret file specified")
	}

	log.Println("fetching configuration from ", server)
	resp, err := httpRequest("GET", server, nil)
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
	c := make(chan struct{})
	for g, tg := range config {
		for t, target := range tg {
			runProbe(g, t, &target)
		}
	}
	<-c
}
