package echostatuspage

import (
	"bytes"
	"net/http"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"

	"github.com/trafficstars/statuspage"
)

// StatusJSON is a handler for framework "echo" to display all the metrics in JSON format
func StatusJSON(ctx echo.Context) error {
	buf := new(bytes.Buffer)
	err := statuspage.WriteMetricsJSON(buf)
	if err != nil {
		logrus.Errorf(`cannot print the status page (JSON): %v`, err)
	}
	ctx.Response().Header().Set("Content-Type", `application/json`)
	return ctx.String(http.StatusOK, string(buf.Bytes()))
}

// StatusPrometheus is a handler for framework "echo" to display all the metrics in Prometheus format
func StatusPrometheus(ctx echo.Context) error {
	buf := new(bytes.Buffer)
	err := statuspage.WriteMetricsPrometheus(buf)
	if err != nil {
		logrus.Errorf(`cannot print the status page (for Prometheus): %v`, err)
	}
	ctx.Response().Header().Set("Content-Type", string(statuspage.PrometheusFormat))
	return ctx.String(http.StatusOK, string(buf.Bytes()))
}
