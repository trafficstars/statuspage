[![Build Status](https://travis-ci.org/trafficstars/statuspage.svg?branch=master)](https://travis-ci.org/trafficstars/statuspage)
[![go report](https://goreportcard.com/badge/github.com/trafficstars/statuspage)](https://goreportcard.com/report/github.com/trafficstars/statuspage)
[![GoDoc](https://godoc.org/github.com/trafficstars/statuspage?status.svg)](https://godoc.org/github.com/trafficstars/statuspage)
[![GoDoc (for framework "echo")](https://godoc.org/github.com/trafficstars/statuspage/handler/echostatuspage?status.svg)](https://godoc.org/github.com/trafficstars/statuspage/handler/echostatuspage)

Description
===========

The package allows to easily export application metrics ([github.com/trafficstars/metrics](https://github.com/trafficstars/metrics)) to prometheus.

Examples
==========

Generic case
------------

```go
package main

import (
    "fmt"
    "math/rand"
    "net/http"

    "github.com/trafficstars/metrics"
    "github.com/trafficstars/statuspage"
)

func hello(w http.ResponseWriter, r *http.Request) {
    answerInt := rand.Intn(10)
        
    // just a metric
    metrics.Count(`hello`, metrics.Tags{`answer_int`: answerInt}).Increment()
        
    fmt.Fprintf(w, "Hello world! The answerInt == %v\n", answerInt)
}

func sendMetrics(w http.ResponseWriter, r *http.Request) {
    statuspage.WriteMetricsPrometheus(w)
}

func main() {
    http.HandleFunc("/", hello)
    http.HandleFunc("/metrics.prometheus", sendMetrics) // here we export metrics for prometheus
    http.ListenAndServe(":8000", nil)
}
```

```sh
$ curl http://localhost:8000/
Hello world! The answerInt == 1

$ curl http://localhost:8000/
Hello world! The answerInt == 7

$ curl http://localhost:8000/
Hello world! The answerInt == 7

$ curl -s http://localhost:8000/metrics.prometheus | grep hello
# TYPE metrics_hello counter
metrics_hello{answer_int="1"} 1
metrics_hello{answer_int="7"} 2
```

Framework "echo"
----------------

The same as above, but just use our handler: 
```go
// import "github.com/trafficstars/statuspage/handler/echostatuspage"

r := echo.New()
r.GET("/status.prometheus", echostatuspage.StatusPrometheus)
```