package main

type Config struct {
	Main struct {
		Title      string
		Maintainer string
		Listen     string
		Secrets    string
	}
	DB      DB
	Probes  map[string]Probe
	Workers map[string]Worker
	Targets map[string]TargetGroup
}

type DB struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
}

type Probe struct {
	Interval int
}

type Worker struct {
	Hostname string
	Display  string
}

type TargetGroup map[string]Target

type Target struct {
	Title    string
	Probe    string
	Interval int
	Count    int
	Host     string
	Workers  []string
}

type Result []float64

type ResultGroup map[string]Result
