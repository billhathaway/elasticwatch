package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gopkg.in/olivere/elastic.v3"
)

const defaultIndex = "_all"

// Policy represents a monitoring policy
type Policy struct {
	// Name of policy
	Name string `json:"name"`
	// Query is the Elastic Search query to perform
	Query string `json:"query"`
	// Index is the Elastic Search index
	Index string `json:"index"`
	// MinCount is the min number of results needed to be in compliance
	MinCount int64 `json:"min_count"`
	// MaxCount is the number not to exceed to be in compliance, set to -1 if not needed
	MaxCount int64 `json:"max_count"`
	// FrequencySeconds is how often to run the query
	FrequencySeconds int `json:"polling_secs"`
	// Handlers are notification/resolution methods
	Handlers []string `json:"handlers"`
	// triggered indicates if policy is currently being violated
	triggered bool
	// triggeredAt is the time the current violation was triggered
	triggeredAt time.Time
	// lastRan is the last time the search was ran
	lastRan     time.Time
	violationID string
	// err is whether an error was returned on last execution
	err error
}

// String pretty prints the policy state
func (p Policy) String() string {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("Name: %s\n", p.Name))
	buf.WriteString(fmt.Sprintf("Query: %s\n", p.Query))
	buf.WriteString(fmt.Sprintf("Index: %s\n", p.Index))
	buf.WriteString(fmt.Sprintf("FrequencySecs: %d\n", p.FrequencySeconds))
	buf.WriteString(fmt.Sprintf("Handlers: %v\n", p.Handlers))
	buf.WriteString(fmt.Sprintf("Last ran: %s\n", p.lastRan))
	if p.err != nil {
		buf.WriteString(fmt.Sprintf("Query status: %s\n", p.err))
	} else {
		buf.WriteString("Query status: ok\n")
	}
	buf.WriteString(fmt.Sprintf("Violation status: %v\n", p.triggered))
	if p.triggered {
		buf.WriteString(fmt.Sprintf("Violation since: %s\n", p.triggeredAt))
		buf.WriteString(fmt.Sprintf("Violation ID: %s\n", p.violationID))
	}
	return buf.String()
}

func (c *controller) policyWorker(p *Policy) {
	// if only MinCount is set, set MaxCount to -1 so we ignore it
	if p.MaxCount == 0 && p.MinCount > 0 {
		p.MaxCount = -1
		c.logf("event=set_default policy=%q max_count=-1\n", p.Name)
	}
	// assume logstash based indexes if not specified
	if p.Index == "" {
		p.Index = defaultIndex
		c.logf("event=set_default policy=%q index=%q\n", p.Name, p.Index)
	}
	ticker := time.NewTicker(time.Duration(p.FrequencySeconds) * time.Second)
	c.logf("event=start_worker policy=%q poll_frequency=%d\n", p.Name, p.FrequencySeconds)
	qs := p.Query
	query := elastic.NewQueryStringQuery(qs)
	for {
		c.logf("event=query_start policy=%q index=%s query=%q\n", p.Name, p.Index, qs)
		searchResult, err := c.client.Search().Index(p.Index).Query(query).SearchType("count").Do()
		p.lastRan = time.Now()
		if err != nil {
			p.err = err
			c.logf("event=es_query policy=%q error=%q", p.Name, err)
			<-ticker.C
			continue
		} else {
			p.err = nil
			c.logf("event=es_query policy=%q status=ok rt=%dms", p.Name, searchResult.TookInMillis)
		}

		count := searchResult.TotalHits()
		if count < p.MinCount || (count > p.MaxCount && p.MaxCount != -1) {
			msg := fmt.Sprintf("event=violation policy=%q events=%d min=%d max=%d", p.Name, count, p.MinCount, p.MaxCount)
			c.logf(msg + "\n")
			// if are already in a violation, don't send new notifications
			if p.triggered {
				c.logf("event=violation_extended policy=%q violation_start=%s\n", p.Name, p.triggeredAt)
			} else {
				// new violation, track it and send notifications
				p.violationID = p.generateID()
				p.triggered = true
				p.triggeredAt = time.Now()
				p.runHandlers(c, "violation")
			}
			// not in violation state
		} else {
			// if previous state was in violation, resolve it
			if p.triggered {
				p.triggered = false
				msg := fmt.Sprintf("event=resolved policy=%q events=%d min=%d max=%d duration=%s", p.Name, count, p.MinCount, p.MaxCount, time.Since(p.triggeredAt))
				c.logf(msg + "\n")
				p.runHandlers(c, "resolved")
				// ok state previously and ok state now
			} else {
				c.logf("event=policy_ok policy=%q events=%d min=%d max=%d\n", p.Name, count, p.MinCount, p.MaxCount)
			}
		}
		<-ticker.C
	}
}

func (p *Policy) runHandlers(c *controller, status string) {
	for _, handler := range p.Handlers {
		c.logf("event=trigger_handler mode=%s policy=%q id=%s handler=%s\n", status, p.Name, p.violationID, handler)
		err := c.handlers[handler].handle(status, p.violationID, p.Name)
		if err != nil {
			c.logf("event=handler_error mode=%s policy=%q handler=%s err=%q\n", status, p.Name, handler, err)
		}
	}
}

func (p Policy) generateID() string {
	seed := []byte(fmt.Sprintf("%d%s", time.Now().UnixNano(), p.Name))
	data := sha256.Sum256(seed)
	return hex.EncodeToString(data[:6])
}
