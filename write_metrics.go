package statuspage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/demdxx/gocast"
	"github.com/fatih/structs"
	prometheusModels "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/trafficstars/metrics"
)

const (
	// PrometheusFormat is a constant that defines which prometheus format will be used
	// see also: https://github.com/prometheus/common/blob/6fb6fce6f8b75884b92e1889c150403fc0872c5e/expfmt/expfmt.go#L27
	PrometheusFormat = expfmt.FmtText
)

var (
	// slice of functions which returns custom metrics (see `AddCustomMetricsHook()`)
	customMetricsHooks = []func() map[string]interface{}{}
)

// getStatus is a helper that collects all the metrics that should be showed from
// * internal logic: it calls different `runtime.*()` functions
// * metrics registry of module "github.com/trafficstars/metrics"
// * custom metrics which could be added via `AddCustomMetricsHook()`
func getStatus() map[string]interface{} {
	result := map[string]interface{}{}

	// Getting metrics from the registry (see "github.com/trafficstars/metrics")
	result[`metrics`] = metrics.List()

	// Getting obvious metrics from "runtime"
	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)
	result[`mem`] = memStats

	result[`num_goroutine`] = runtime.NumGoroutine()
	result[`num_cgo_call`] = runtime.NumCgoCall()
	result[`num_cpu`] = runtime.NumCPU()
	result[`golang_version`] = runtime.Version()

	result[`default_tags`] = metrics.GetDefaultTags().String()

	// Getting custom metrics (see `AddCustomMetricsHook()`)
	for _, hook := range customMetricsHooks {
		for k, v := range hook() {
			result[k] = v
		}
	}

	return result
}

// fixPrometheusKey is required to get rid of characters which prometheus doesn't support in a metric key
//
// See: https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
//
// We used StatsD previously, so we have a lot of metrics with "." in a key name. That's why we replace "." with
// "_" here (prometheus doesn't support "." in keys/labels).
func fixPrometheusKey(k string) string {
	return strings.Replace(k, `.`, `_`, -1)
}

// encoder is just a helper interface for writeMetricsPrometheus
type encoder interface {
	Encode(*prometheusModels.MetricFamily) error
}

// writeTimeMetricPrometheus is just a helper function for writeMetricsPrometheus
//
// writes a time.Time metric via encoder
func writeTimeMetricPrometheus(encoder encoder, k string, v time.Time) {
	logger.IfError(encoder.Encode(&prometheusModels.MetricFamily{
		Name: &[]string{k}[0],
		Type: &[]prometheusModels.MetricType{prometheusModels.MetricType_GAUGE}[0],
		Metric: []*prometheusModels.Metric{
			{
				Gauge: &prometheusModels.Gauge{
					Value: &[]float64{gocast.ToFloat64(v.Unix())}[0],
				},
			},
		},
	}))
}

// writeFloat64MetricPrometheus is just a helper function for writeMetricsPrometheus
//
// writes an integer or float metric via encoder
func writeFloat64MetricPrometheus(encoder encoder, k string, v interface{}) {
	logger.IfError(encoder.Encode(&prometheusModels.MetricFamily{
		Name: &[]string{k}[0],
		Type: &[]prometheusModels.MetricType{prometheusModels.MetricType_GAUGE}[0],
		Metric: []*prometheusModels.Metric{
			{
				Gauge: &prometheusModels.Gauge{
					Value: &[]float64{gocast.ToFloat64(v)}[0],
				},
			},
		},
	}))
}

// registryAggregativeMetric is just a helper interface for writeMetricsPrometheus
type registryAggregativeMetric interface {
	metrics.Metric

	GetValuePointers() *metrics.AggregativeValues
	GetAggregationPeriods() []metrics.AggregationPeriod
}

// writeRegistryAggregativeMetric is just a helper function for writeRegistryMetricsPrometheus
//
// writes an aggregative registry metric from package (see "github.com/trafficstars/metrics") via encoder
//
// aggregativeMetrics -- is an output map
func addRegistryAggregativeMetricToMap(
	prefix string,
	metric registryAggregativeMetric,
	labels []*prometheusModels.LabelPair,
	aggregativeMetrics map[string][]*prometheusModels.Metric,
) {

	// Aggregative metrics has multiple aggregation periods (see `SetAggregationPeriods` of
	// "github.com/trafficstars/metrics"). Here we get a slice of values per aggregation period.
	values := metric.GetValuePointers()

	// Just not to duplicate the same code a lot of times we create this temporary function here (it will be used below)
	// This function just adds a metric to the output map "aggregativeMetrics".
	addAggregativeMetric := func(key string, v float64) {
		aggregativeMetrics[key] = append(aggregativeMetrics[key], &prometheusModels.Metric{
			Label: labels,
			Gauge: &prometheusModels.Gauge{
				Value: &[]float64{v}[0],
			},
		})
	}

	// Just not to duplicate the same code a lot of times we create this temporary function here (it will be used below)
	// This function just add metrics to the output map "aggregativeMetrics" for every aggregation type (min, avg,
	// percentile99 and so on)
	considerValue := func(label string) func(data *metrics.AggregativeValue) {
		return func(data *metrics.AggregativeValue) {
			if data.Count == 0 {
				return
			}
			addAggregativeMetric(prefix+`_`+label+`_count`, float64(data.Count))
			addAggregativeMetric(prefix+`_`+label+`_min`, data.Min.Get())
			addAggregativeMetric(prefix+`_`+label+`_avg`, data.Avg.Get())
			addAggregativeMetric(prefix+`_`+label+`_max`, data.Max.Get())
			addAggregativeMetric(prefix+`_`+label+`_sum`, data.Sum.Get())
			aggregativeStatistics := data.AggregativeStatistics
			if aggregativeStatistics == nil {
				return
			}
			percentiles := aggregativeStatistics.GetPercentiles([]float64{0.01, 0.1, 0.5, 0.9, 0.99})
			addAggregativeMetric(prefix+`_`+label+`_per1`, *percentiles[0])
			addAggregativeMetric(prefix+`_`+label+`_per10`, *percentiles[1])
			addAggregativeMetric(prefix+`_`+label+`_per50`, *percentiles[2])
			addAggregativeMetric(prefix+`_`+label+`_per90`, *percentiles[3])
			addAggregativeMetric(prefix+`_`+label+`_per99`, *percentiles[4])
		}
	}

	// Just add all the metrics for every aggregation period (total, 5sec, so on) and aggregation type (min, avg, so on)
	//
	// Aggregation period "last" is the current instance value (without aggregation, just the last value).

	values.Last.LockDo(considerValue(`last`))
	if len(values.ByPeriod) > 0 {
		values.ByPeriod[0].LockDo(considerValue(metrics.GetBaseAggregationPeriod().String()))
	}
	for idx, period := range metric.GetAggregationPeriods() {
		byPeriod := values.ByPeriod
		if idx >= len(byPeriod) {
			break
		}
		byPeriod[idx].LockDo(considerValue(period.String()))
	}
	values.Total.LockDo(considerValue(`total`))
}

// encodeMetrics is just a helper function for writeRegistryMetricsPrometheus
//
// writes non-aggregative registry metrics (see "github.com/trafficstars/metrics") via encoder
func encodeMetrics(encoder encoder, prefix string, metrics map[string][]*prometheusModels.Metric, metricType prometheusModels.MetricType) {
	for key, subMetrics := range metrics {
		logger.IfError(encoder.Encode(&prometheusModels.MetricFamily{
			Name:   &[]string{fixPrometheusKey(prefix + key)}[0],
			Type:   &[]prometheusModels.MetricType{metricType}[0],
			Metric: subMetrics,
		}))
	}
}

// writeRegistryMetricsPrometheus is just a helper function for writeMetricsPrometheus
//
// writes registry metrics (see "github.com/trafficstars/metrics") via encoder
func writeRegistryMetricsPrometheus(encoder encoder, prefix string, v []metrics.Metric) {
	// A slice of registry metrics (likely received via `List` of "github.com/trafficstars/metrics")

	countMetrics := map[string][]*prometheusModels.Metric{}
	gaugeMetrics := map[string][]*prometheusModels.Metric{}
	aggregativeMetrics := map[string][]*prometheusModels.Metric{}

	defaultTags := metrics.GetDefaultTags()

	// Collecting all the metrics from the slice to maps: countMetrics, gaugeMetrics and aggregativeMetrics

	for _, metricI := range v {
		key := metricI.GetName()

		// Prepare labels (it's called "tags" in package "github.com/trafficstars/metrics")

		var labels []*prometheusModels.LabelPair
		tags := metricI.GetTags()
		tags.Each(func(k string, v interface{}) bool {
			value := metrics.TagValueToString(v)
			if !utf8.ValidString(value) {
				value = base64.StdEncoding.EncodeToString([]byte(value))
			}
			labels = append(labels, &prometheusModels.LabelPair{
				Name:  &[]string{k}[0],
				Value: &[]string{value}[0],
			})
			return true
		})

		defaultTags.Each(func(k string, v interface{}) bool {
			value := metrics.TagValueToString(v)
			if !utf8.ValidString(value) {
				value = base64.StdEncoding.EncodeToString([]byte(value))
			}
			labels = append(labels, &prometheusModels.LabelPair{
				Name:  &[]string{k}[0],
				Value: &[]string{value}[0],
			})
			return true
		})

		// Detect registry metric type and add it to and appropriate map: countMetrics, gaugeMetrics or
		// aggregativeMetrics

		switch metricI.GetType() {
		case metrics.TypeTimingFlow, metrics.TypeTimingBuffered, metrics.TypeTimingSimple,
			metrics.TypeGaugeAggregativeFlow, metrics.TypeGaugeAggregativeBuffered, metrics.TypeGaugeAggregativeSimple:
			addRegistryAggregativeMetricToMap(key, metricI.(registryAggregativeMetric), labels, aggregativeMetrics)

		case metrics.TypeCount:
			countMetrics[key] = append(countMetrics[key], &prometheusModels.Metric{
				Label: labels,
				Counter: &prometheusModels.Counter{
					Value: &[]float64{metricI.GetFloat64()}[0],
				},
			})
		case metrics.TypeGaugeFloat64, metrics.TypeGaugeInt64,
			metrics.TypeGaugeFloat64Func, metrics.TypeGaugeInt64Func:
			gaugeMetrics[key] = append(gaugeMetrics[key], &prometheusModels.Metric{
				Label: labels,
				Gauge: &prometheusModels.Gauge{
					Value: &[]float64{metricI.GetFloat64()}[0],
				},
			})
		default:
			logger.Error(errors.New("unknown metric type (registry case)"))
			// TODO: do something here
		}
	}

	// Writing all the collected metrics (in the maps) via the encoder

	encodeMetrics(encoder, prefix, countMetrics, prometheusModels.MetricType_COUNTER)
	encodeMetrics(encoder, prefix, gaugeMetrics, prometheusModels.MetricType_GAUGE)
	encodeMetrics(encoder, prefix, aggregativeMetrics, prometheusModels.MetricType_GAUGE)
}

// writeMetricsPrometheus writes all the metrics listed in map "m" via encoder "encoder"
//
// "prefix" is used as a prefix for the metric label/key.
func writeMetricsPrometheus(encoder encoder, prefix string, m map[string]interface{}) {
	for k, vI := range m {
		if registryMetric, ok := vI.(metrics.Metric); ok {
			// The only way to work with registry metrics ("github.com/trafficstars/metrics") is pass them as a slice,
			// ATM. Sorry.
			_ = registryMetric
			logger.Error(errors.New("registry metrics outside of a slice are not implemented, yet"))
			// TODO: implement this case
			continue
		}

		// Detect the value type of the metric and encode it via "encoder"
		switch v := vI.(type) {
		case time.Time:
			writeTimeMetricPrometheus(encoder, prefix+k, v)

		case int, int32, uint32, int64, uint64, float32, float64:
			writeFloat64MetricPrometheus(encoder, prefix+k, v)

		case []metrics.Metric:
			writeRegistryMetricsPrometheus(encoder, prefix+k+"_", v)

		case map[string]interface{}:
			writeMetricsPrometheus(encoder, prefix+k+"_", v) // recursive walk in

		case *runtime.MemStats:
			writeMetricsPrometheus(encoder, prefix+k+"_", structs.Map(v)) // recursive walk in

		default:
			logger.Error(errors.New("unknown metric type (simple case)"))
			// TODO: do something here
		}
	}
}

// WriteMetricsPrometheus write all the metrics via writer in prometheus format.
func WriteMetricsPrometheus(writer io.Writer) error {
	// Just create the prometheus encoder...
	prometheusEncoder := expfmt.NewEncoder(writer, PrometheusFormat)

	// ... Get all the metrics...
	metrics := getStatus()

	// ... And write them via the encoder
	writeMetricsPrometheus(prometheusEncoder, ``, metrics)
	return nil
}

// WriteMetricsJSON write all the metrics via writer in JSON format.
func WriteMetricsJSON(writer io.Writer) error {
	return json.NewEncoder(writer).Encode(getStatus())
}

// AddCustomMetricsHook adds a new hook that will be called everytime to collect additional metrics when function
// WriteMetricsPrometheus or WriteMetricsJSON is called.
//
// The hook should return map of "string to interface{}" where the "string" is a metric key and "interface{}" is the
// value.
func AddCustomMetricsHook(hook func() map[string]interface{}) {
	customMetricsHooks = append(customMetricsHooks, hook)
}
