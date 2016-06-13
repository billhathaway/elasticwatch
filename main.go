package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"gopkg.in/olivere/elastic.v3"
)

type (
	config struct {
		Address  string                       `json:"listen_address"`
		ESURL    string                       `json:"elasticsearch_url"`
		Policies []Policy                     `json:"policies"`
		Handlers map[string]map[string]string `json:"handlers"`
	}

	controller struct {
		client   *elastic.Client
		config   *config
		handlers map[string]handler
		logger   *log.Logger
	}
)

func testConfig() *config {
	return &config{
		Address: "127.0.0.1:8888",
		ESURL:   "http://localhost:9200/",
		Handlers: map[string]map[string]string{
			"stdout":    {},
			"pagerduty": {"apikey": "abcd1234"},
		},
		Policies: []Policy{
			{
				Name:             "apache hits in last hour",
				Query:            "_type:logs AND path:/var/log/apache2/access_log AND @timestamp:{now-1h TO now}",
				MinCount:         100,
				MaxCount:         -1,
				FrequencySeconds: 300,
				Handlers:         []string{"pagerduty"},
			},
		},
	}
}

// validateConfig validates configuration
func validateConfig(r io.Reader) (*config, error) {
	decoder := json.NewDecoder(r)
	var cfg config
	err := decoder.Decode(&cfg)
	if err != nil {
		return nil, err
	}
	for _, policy := range cfg.Policies {
		if policy.FrequencySeconds == 0 {
			return nil, fmt.Errorf("Policy %s polling frequency needs to be set", policy.Name)
		}
		for _, h := range policy.Handlers {
			if _, ok := cfg.Handlers[h]; !ok {
				return nil, fmt.Errorf("Policy %s specified a handler %s but it wasn't defined", policy.Name, h)
			}
		}
	}
	if cfg.ESURL == "" {
		cfg.ESURL = elastic.DefaultURL
	}
	return &cfg, nil
}

func newController(cfg *config) (*controller, error) {
	c := controller{
		config: cfg,
		logger: log.New(os.Stdout, "elasticwatch ", log.LstdFlags),
	}
	var err error
	c.client, err = elastic.NewClient(elastic.SetURL(cfg.ESURL))
	if err != nil {
		return nil, err
	}
	c.handlers = make(map[string]handler)
	for name, settings := range cfg.Handlers {
		n, err := newHandler(name, settings)
		if err != nil {
			return nil, err
		}
		c.handlers[name] = n
	}
	for _, policy := range cfg.Policies {
		for _, h := range policy.Handlers {
			if _, ok := c.handlers[h]; !ok {
				return nil, fmt.Errorf("Policy %s specified a handler %s but it wasn't defined", policy.Name, h)
			}
		}
	}
	for i := range c.config.Policies {
		go c.policyWorker(&c.config.Policies[i])
	}
	return &c, nil
}

func (c *controller) logf(format string, a ...interface{}) {
	c.logger.Printf(format, a...)
}

// statusHandler reports the status of all policies
func (c *controller) statusHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Policies")
	for _, p := range c.config.Policies {
		fmt.Fprintln(w, p)
		fmt.Fprintln(w)
	}
}

func main() {
	configFile := flag.String("f", "config.json", "config file to load")
	flag.Parse()
	fh, err := os.Open(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := validateConfig(fh)
	if err != nil {
		log.Fatal(err)
	}
	fh.Close()
	c, err := newController(cfg)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", c.statusHandler)
	log.Fatal(http.ListenAndServe(c.config.Address, nil))
}
