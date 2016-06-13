elasticwatch
==
Elasticwatch (EW) performs queries against Elastic Search, firing off violation and resolve events when they exceed/return to constraints.

There are 2 main types used by the Elasticwatch service:
  1. Policies - define an Elastic Search query and the minimum or maximum events detected by that query
  2. Handlers - logic for handling event state transitions


Running:  
  `elasticwatch`   (use default configuration file config.json)
  or  
  `elasticwatch -f /path/to/config.json`

  If there is a problem starting up, a message will go to stderr, otherwise log messages during runtime will be sent to stdout.

Configuration:
  EW uses a JSON configuration file.  

  Below is an example where the service listens on port 127.0.0.1:8080 to give status reports, and performs search requests against an Elastic Search instance on http://127.0.0.1:9200

  There are two handlers defined at the top level:  

  shell - runs the command specified and passes it 3 parameters
  * status - "violation" or "resolved"
  * id - internal guid for the event
  * policy name


  hipchat - uses the HipChat notifications API to send a message
  properties required:
  * apikey  
  * room (name or id)  


  Two policies are defined.  Each policy must have the following properties:
  * name  
  * query  
  * min_count and/or max_count  
  * polling_secs  


  index is an optional property, if not defined, it will default to using 'logstash-*'.

See the config.json.example file in the repo
```
{
  "listen_address": "127.0.0.1:8080",
  "elasticsearch_url": "http://127.0.0.1:9200",
  "handlers": {
    "shell": {
      "command": "/usr/local/bin/alert"
    },
   "hipchat": {
     "apikey": "abc123",
     "room": "devops"
   }
  },
  "policies": [{
    "name": "apache requests",
    "query": "path:/var/log/apache2/access_log AND @timestamp:{now-1h TO now}",
    "min_count": 100,
    "polling_secs": 3600,
    "handlers": ["shell"]
  }, {
    "name": "crawl requests",
    "query": "path:/var/log/crawler AND @timestamp:{now-5m TO now}",
    "min_count": 10,
    "max_count": 100,
    "polling_secs": 300,
    "handlers": ["shell", "hipchat"]
  }]
}
```
