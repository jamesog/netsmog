package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/handlers"
	influxdb "github.com/influxdb/influxdb/client"
	"golang.org/x/crypto/bcrypt"
)

// TODO(jamesog):
// Generate per-worker configs on-demand
// Config file with shared secrets for workers
// HTTP interface, providing:
//  * UI for viewing graphs
//  * worker API (fetch configs, post results)

var config Config

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

func uiHandler() http.Handler {
	ui := func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("template/index.html.tmpl"))
		tmpl.Execute(w, config.Targets)
	}

	return http.HandlerFunc(ui)
}

func workerHandler(c *map[string]TargetGroup, dbClient *influxdb.Client) http.Handler {
	h := func(w http.ResponseWriter, r *http.Request) {
		worker := r.Header.Get("Worker")
		auth := r.Header.Get("Authorisation")
		// TODO(jamesog): Tally this with the Authorisation header
		switch {
		case r.Method == "GET":
			// Pass the worker a JSON object with its tasks
			log.Println("Received request from", worker)
			if err := checkAuthorisation(worker, auth); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			// Query the config for all targets this worker is a member of
			// and create a new struct to pass to json.Marshal()
			// TODO(jamesog): This only checks the "workers" virtual group
			// It should also check inside each target
			workerTargets := make(map[string]TargetGroup)
			for g, group := range *c {
				wg := make(map[string]Target)
				for _, w := range group["meta"].Workers {
					if w == worker {
						for t, target := range group {
							if t == "meta" {
								continue
							}
							wg[t] = target
						}
					}
				}
				if len(group["meta"].Workers) == 0 {
					for t, target := range group {
						if t == "meta" {
							continue
						}
						wg[t] = target
					}
				}
				workerTargets[g] = wg
			}
			t, err := json.Marshal(workerTargets)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write(t)

		case r.Method == "POST":
			// Receive JSON and write the data to the DB
			log.Println("Received results from", worker)
			if err := checkAuthorisation(worker, auth); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Println("Error reading request from client:", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			var results map[string]ResultGroup
			err = json.Unmarshal(body, &results)
			if err != nil {
				log.Println("Error unmarshalling results:", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			var series []*influxdb.Series
			for g, tg := range results {
				for t, target := range tg {
					for i := 0; i < len(target); i++ {
						s := &influxdb.Series{}
						s.Name = fmt.Sprintf("%s.%s", g, t)
						s.Columns = []string{"worker", "value"}
						s.Points = [][]interface{}{
							[]interface{}{worker, target[i]},
						}
						series = append(series, s)
					}
				}
			}
			err = dbClient.WriteSeries(series)
			if err != nil {
				log.Println("Error writing series:", err)
			}
			w.WriteHeader(http.StatusOK)
		}
	}

	return http.HandlerFunc(h)
}

func checkAuthorisation(worker, enchash string) error {
	buf, err := ioutil.ReadFile(config.Main.Secrets)
	if err != nil {
		log.Println("Error reading secrets:", err)
		return err
	}
	var secrets map[string]string
	if err := toml.Unmarshal(buf, &secrets); err != nil {
		log.Println("Couldn't unmarshal secrets:", err)
	}
	want := []byte(fmt.Sprintf("%s:%s", worker, secrets[worker]))
	hash, err := base64.URLEncoding.DecodeString(enchash)
	if err != nil {
		log.Printf("Could not decode hash:", err)
		return err
	}
	err = bcrypt.CompareHashAndPassword(hash, want)
	if err != nil {
		log.Printf("SECURITY: %s authorisation is incorrect\n", worker)
		return errors.New("Security fail")
	}
	return nil
}

func parseConfig(file string) {
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}
	if err := toml.Unmarshal(buf, &config); err != nil {
		panic(err)
	}
}

func main() {
	var confFile = flag.String("config", "config.toml", "config file, in TOML format")
	flag.Parse()
	parseConfig(*confFile)

	// Listen for SIGHUP to notify us to re-read the config file.
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for {
			sig := <-sighup
			log.Printf("Received %s, reloading config.\n", sig)
			parseConfig(*confFile)
		}
	}()

	// fmt.Printf("Config: %+v\n", config)
	log.Printf("Netsmog instance for %s\n", config.Main.Title)
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

	ui := uiHandler()
	w := workerHandler(&config.Targets, dbClient)
	http.Handle("/", handlers.LoggingHandler(os.Stdout, ui))
	http.Handle("/favicon.ico", handlers.LoggingHandler(os.Stdout, http.NotFoundHandler()))
	http.Handle("/worker", handlers.LoggingHandler(os.Stdout, w))
	log.Println("Listening on", config.Main.Listen)
	if err := http.ListenAndServe(config.Main.Listen, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
