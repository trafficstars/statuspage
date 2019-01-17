package statuspage

import (
	"bytes"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/labstack/echo"

	"github.com/trafficstars/statuspage"
)

var (
	customMetricsHooks = []func() map[string]interface{}{}
)

func StatusJSON(ctx echo.Context) error {
	buf := new(bytes.Buffer)
	err := statuspage.WriteMetricsJSON(buf)
	if err != nil {
		logrus.Errorf(`cannot print the status page (JSON): %v`, err)
	}
	ctx.Response().Header().Set("Content-Type", `appliation/json`)
	return ctx.String(http.StatusOK, string(buf.Bytes()))
}

func StatusPrometheus(ctx echo.Context) error {
	buf := new(bytes.Buffer)
	err := statuspage.WriteMetricsPrometheus(buf)
	if err != nil {
		logrus.Errorf(`cannot print the status page (for Prometheus): %v`, err)
	}
	ctx.Response().Header().Set("Content-Type", string(statuspage.PrometheusFormat))
	return ctx.String(http.StatusOK, string(buf.Bytes()))
}
