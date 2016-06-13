package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
)

type (
	handler interface {
		handle(status, id, msg string) error
	}
	pagerduty struct{ apikey string }
	shell     struct{ command string }
	hipChat   struct {
		apikey   string
		endpoint string
		room     string
		from     string
	}
)

func (s shell) handle(status, id, msg string) error {
	cmd := exec.Command(s.command, status, id, msg)
	return cmd.Run()
}

func (pagerduty) handle(status, id, msg string) error {
	fmt.Printf("pagerduty type=%s id=%s %s\n", status, id, msg)
	return nil
}

func (h hipChat) handle(status, id, msg string) error {
	url := fmt.Sprintf("%s/v2/room/%s/notification?auth_token=%s", h.endpoint, h.room, h.apikey)
	body := fmt.Sprintf("elasticwatch status=%s policy=%q id=%s", status, msg, id)
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("expected response code 200 or 204, received %d", resp.StatusCode)
	}
	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

// newHandler is a factory method that checks for any necessary configuration elements
func newHandler(name string, config map[string]string) (handler, error) {
	switch name {
	case "pagerduty":
		apikey, ok := config["apikey"]
		if !ok {
			return nil, errors.New("apikey must be defined")
		}
		return pagerduty{apikey: apikey}, nil
	case "shell":
		command, ok := config["command"]
		if !ok {
			return nil, errors.New("command must be defined")
		}
		return shell{command: command}, nil
	case "hipchat":
		apikey, ok := config["apikey"]
		if !ok {
			return nil, errors.New("apikey must be defined")
		}
		room, ok := config["room"]
		if !ok {
			return nil, errors.New("room must be defined")
		}
		endpoint, ok := config["endpoint"]
		if !ok {
			endpoint = "https://api.hipchat.com/"
		}
		return hipChat{
			apikey:   apikey,
			endpoint: endpoint,
			room:     room,
		}, nil
	default:
		return nil, fmt.Errorf("unknown handler type %s", name)
	}
}
