package statuspage

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	prometheusModels "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
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
	// Check if the format is correct

	metrics.SetDefaultTags(metrics.Tags{
		`tag0`: true,
		`tag1`: true,
	})

	metric0 := metrics.Count(`test`, metrics.Tags{
		`tag1`:              true,
		`invalidUTF8String`: string([]byte{0xff, 0xfe, 0xfd}),
		`hackyString`:       "dummyValue}\ninjectedMetric{",
	})
	metric0.Increment()

	fastTags := metrics.NewFastTags().
		Set(`tag1`, true).
		Set(`tag2`, true)

	metric1 := metrics.Count(`test`, fastTags)
	metric1.Add(2)

	assert.Equal(t, `test,tag2=true`, string(metric1.GetKey()))

	status := map[string]interface{}{`metrics`: metrics.List()}

	writeMetricsPrometheus(&testPrometheusEncoder{t: t}, ``, status)

	buf := new(bytes.Buffer)
	realPrometheusEncoder := expfmt.NewEncoder(buf, PrometheusFormat)
	writeMetricsPrometheus(realPrometheusEncoder, ``, status)

	lines := strings.Split(string(buf.Bytes()), "\n")
	assert.Equal(t, 4, len(lines))

	// Check if keys are correct
	for _, line := range lines {
		if strings.HasPrefix(line, `#`) {
			continue
		}
		switch {
		case strings.HasSuffix(line, ` 1`):
			assert.Equal(t, `metrics_test{hackyString="dummyValue}\ninjectedMetric{",invalidUTF8String="//79",tag0="true",tag1="true"} 1`, line)
		case strings.HasSuffix(line, ` 2`):
			assert.Equal(t, `metrics_test{tag0="true",tag1="true",tag2="true"} 2`, line)
		}
	}
}
