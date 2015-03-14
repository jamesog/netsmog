package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	influxdb "github.com/influxdb/influxdb/client"
)

// TODO(jamesog):
// Generate per-worker configs on-demand
// Config file with shared secrets for workers
// HTTP interface, providing:
//  * UI for viewing graphs
//  * worker API (fetch configs, post results)

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
			Host:     config.DB.Host + ":" + fmt.Sprintf("%d", config.DB.Port),
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
