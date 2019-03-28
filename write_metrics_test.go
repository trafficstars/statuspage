package statuspage

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	prometheusModels "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/trafficstars/metrics"
)

type testPrometheusEncoder struct {
	t *testing.T
}

func (e *testPrometheusEncoder) Encode(family *prometheusModels.MetricFamily) error {
	for _, metric := range family.Metric {
		for _, label := range metric.Label {
			if !utf8.ValidString(*label.Value) {
				e.t.Errorf("An invalid UTF8 string: %v", []byte(*label.Value))
			}
		}
	}
	return nil
}

func TestEncodeMetricsPrometheus(t *testing.T) {
	metrics.Count(`test`, metrics.Tags{
		`invalidUTF8String`: string([]byte{0xff, 0xfe, 0xfd}),
		`hackyString`:       "dummyValue}\ninjectedMetric{",
	})

	status := map[string]interface{}{`metrics`: metrics.List()}

	writeMetricsPrometheus(status, &testPrometheusEncoder{t: t}, ``)

	buf := new(bytes.Buffer)
	realPrometheusEncoder := expfmt.NewEncoder(buf, PrometheusFormat)
	writeMetricsPrometheus(status, realPrometheusEncoder, ``)
	if len(strings.Split(string(buf.Bytes()), "\n")) != 3 {
		t.Errorf("result: %v", string(buf.Bytes()))
	}
}
