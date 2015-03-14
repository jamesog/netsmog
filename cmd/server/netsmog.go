package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/handlers"
	influxdb "github.com/influxdb/influxdb/client"
)

// TODO(jamesog):
// Generate per-worker configs on-demand
// Config file with shared secrets for workers
// HTTP interface, providing:
//  * UI for viewing graphs
//  * worker API (fetch configs, post results)

func results(dbClient *influxdb.Client) {
	results, err := dbClient.Query("select * from www1", "s")
	// fmt.Printf("%q\n", results)
	if err != nil {
		log.Fatalf("Herp derp: %s\n", err)
	}
	for _, series := range results {
		// fmt.Printf("Series %d\n--------\n", i)
		fmt.Printf("Series: %s\n", series.Name)
		for _, column := range series.Columns {
			fmt.Printf("%s\t", column)
		}
		fmt.Println()
		for _, point := range series.Points {
			for _, c := range point {
				switch c := c.(type) {
				case string:
					fmt.Printf("%s\t", c)
				case int:
					fmt.Printf("%d\t", c)
				case float64:
					fmt.Printf("%.02f\t", c)
				}
			}
			fmt.Println()
		}
	}
}

func workerHandler(c *map[string]TargetGroup) http.Handler {
	// Pass the worker a JSON object with its tasks
	json := func(w http.ResponseWriter, r *http.Request) {
		worker := r.Header.Get("Worker")
		// TODO(jamesog): Tally this with the Authorisation header
		log.Println("Received request from", worker)
		// Query the config for all targets this worker is a member of
		// and create a new struct to pass to json.Marshal()
		// TODO(jamesog): This only checks the "workers" virtual group
		// It should also check inside each target
		workerTargets := make(TargetGroup)
		for _, group := range *c {
			for _, w := range group["workers"].Workers {
				if w == worker {
					for t, target := range group {
						if t == "workers" {
							continue
						}
						workerTargets[t] = target
					}
				}
			}
		}
		t, _ := json.Marshal(workerTargets)
		w.Write(t)
	}

	return http.HandlerFunc(json)
}

func main() {
	var confFile = flag.String("config", "config.toml", "config file, in TOML format")
	flag.Parse()
	f, err := os.Open(*confFile)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}
	var config Config
	if err := toml.Unmarshal(buf, &config); err != nil {
		panic(err)
	}

	// fmt.Printf("Config: %+v\n", config)
	fmt.Printf("Netsmog instance for %s\n", config.Main.Title)
	fmt.Printf("This instance is maintained by %s\n\n", config.Main.Maintainer)

	for s, worker := range config.Workers {
		fmt.Printf("Worker: %s\nDisplay: %s\nHostname: %s\n\n", s, worker.Display, worker.Hostname)
	}

	for g, tgroup := range config.Targets {
		fmt.Printf("Target group %s has workers %+v\n", g, tgroup["workers"].Workers)
		for t, target := range tgroup {
			if t == "workers" {
				continue
			}
			fmt.Printf("Group: %s\nTarget: %s\nHost: %s\n\n", g, t, target.Host)
		}
	}

	dbClient, err := influxdb.NewClient(
		&influxdb.ClientConfig{
			Host:     fmt.Sprintf("%s:%d", config.DB.Host, config.DB.Port),
			Username: config.DB.Username,
			Password: config.DB.Password,
			Database: config.DB.Database,
		})
	if err != nil {
		log.Fatalf("Could not create client: %s\n", err)
	}
	if e := dbClient.Ping(); e != nil {
		log.Fatalf("Failed to connect: %s\n", e)
	} else {
		// log.Printf("Connected to %s version %s\n", dbClient.Addr(), v)
		log.Println("Connected to InfluxDB.")
	}

	w := workerHandler(&config.Targets)
	http.Handle("/", handlers.LoggingHandler(os.Stdout, http.NotFoundHandler()))
	http.Handle("/worker", handlers.LoggingHandler(os.Stdout, w))
	log.Println("Listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
