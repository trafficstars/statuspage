package statuspage

import (
	"encoding/base64"
	"encoding/json"
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
	PrometheusFormat = expfmt.FmtText
)

var (
	customMetricsHooks = []func() map[string]interface{}{}
)

func getStatus() map[string]interface{} {
	result := map[string]interface{}{}

	result[`metrics`] = metrics.List()

	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)
	result[`mem`] = memStats

	result[`num_goroutine`] = runtime.NumGoroutine()
	result[`num_cgo_call`] = runtime.NumCgoCall()
	result[`num_cpu`] = runtime.NumCPU()
	result[`golang_version`] = runtime.Version()

	for _, hook := range customMetricsHooks {
		for k, v := range hook() {
			result[k] = v
		}
	}

	return result
}

func fixPrometheusKey(k string) string {
	return strings.Replace(k, `.`, `_`, -1)
}

func writeMetricsPrometheus(m map[string]interface{}, encoder interface {
	Encode(*prometheusModels.MetricFamily) error
}, prefix string) {
	for k, vI := range m {
		if internalMetric, ok := vI.(*metrics.Metric); ok {
			_ = internalMetric
			// TODO: implement this case
			continue
		} else {
			switch v := vI.(type) {
			case time.Time:
				encoder.Encode(&prometheusModels.MetricFamily{
					Name: &[]string{prefix + k}[0],
					Type: &[]prometheusModels.MetricType{prometheusModels.MetricType_GAUGE}[0],
					Metric: []*prometheusModels.Metric{
						&prometheusModels.Metric{
							Gauge: &prometheusModels.Gauge{
								Value: &[]float64{gocast.ToFloat64(v.Unix())}[0],
							},
						},
					},
				})
			case int, int32, uint32, int64, uint64, float32, float64:
				encoder.Encode(&prometheusModels.MetricFamily{
					Name: &[]string{prefix + k}[0],
					Type: &[]prometheusModels.MetricType{prometheusModels.MetricType_GAUGE}[0],
					Metric: []*prometheusModels.Metric{
						&prometheusModels.Metric{
							Gauge: &prometheusModels.Gauge{
								Value: &[]float64{gocast.ToFloat64(v)}[0],
							},
						},
					},
				})
			case map[string]interface{}:
				writeMetricsPrometheus(v, encoder, k+"_")
			case *runtime.MemStats:
				writeMetricsPrometheus(structs.Map(v), encoder, k+"_")
			case []metrics.Metric:
				countMetrics := map[string][]*prometheusModels.Metric{}
				gaugeMetrics := map[string][]*prometheusModels.Metric{}
				timingMetrics := map[string][]*prometheusModels.Metric{}

				for _, metricI := range v {
					key := metricI.GetName()
					var labels []*prometheusModels.LabelPair
					for k, v := range metricI.GetTags() {
						value := metrics.TagValueToString(v)
						if !utf8.ValidString(value) {
							value = base64.StdEncoding.EncodeToString([]byte(value))
						}
						labels = append(labels, &prometheusModels.LabelPair{
							Name:  &[]string{k}[0],
							Value: &[]string{value}[0],
						})
					}
					switch metricI.GetType() {
					case metrics.TypeTimingFlow, metrics.TypeTimingBuffered, metrics.TypeGaugeAggregativeFlow, metrics.TypeGaugeAggregativeBuffered:
						values := metricI.(interface {
							GetValuePointers() *metrics.AggregativeValues
						}).GetValuePointers()

						addTimingMetric := func(key string, v float64) {
							timingMetrics[key] = append(timingMetrics[key], &prometheusModels.Metric{
								Label: labels,
								Gauge: &prometheusModels.Gauge{
									Value: &[]float64{v}[0],
								},
							})
						}

						considerValue := func(label string) func(data *metrics.AggregativeValue) {
							return func(data *metrics.AggregativeValue) {
								if data.Count == 0 {
									return
								}
								addTimingMetric(key+`_`+label+`_count`, float64(data.Count))
								addTimingMetric(key+`_`+label+`_min`, data.Min.Get())
								addTimingMetric(key+`_`+label+`_avg`, data.Avg.Get())
								addTimingMetric(key+`_`+label+`_max`, data.Max.Get())
								if data.AggregativeStatistics != nil {
									percentiles := data.AggregativeStatistics.GetPercentiles([]float64{0.01, 0.1, 0.5, 0.9, 0.99})
									addTimingMetric(key+`_`+label+`_per1`, *percentiles[0])
									addTimingMetric(key+`_`+label+`_per10`, *percentiles[1])
									addTimingMetric(key+`_`+label+`_per50`, *percentiles[2])
									addTimingMetric(key+`_`+label+`_per90`, *percentiles[3])
									addTimingMetric(key+`_`+label+`_per99`, *percentiles[4])
								}
							}
						}

						values.Last.LockDo(considerValue(`last`))
						values.ByPeriod[0].LockDo(considerValue(metrics.GetBaseAggregationPeriod().String()))
						for idx, period := range metricI.(interface {
							GetAggregationPeriods() []metrics.AggregationPeriod
						}).GetAggregationPeriods() {
							values.ByPeriod[idx].LockDo(considerValue(period.String()))
						}
						values.Total.LockDo(considerValue(`total`))
					case metrics.TypeCount:
						countMetrics[key] = append(countMetrics[key], &prometheusModels.Metric{
							Label: labels,
							Counter: &prometheusModels.Counter{
								Value: &[]float64{metricI.GetFloat64()}[0],
							},
						})
					case metrics.TypeGaugeFloat64, metrics.TypeGaugeInt64:
						gaugeMetrics[key] = append(gaugeMetrics[key], &prometheusModels.Metric{
							Label: labels,
							Gauge: &prometheusModels.Gauge{
								Value: &[]float64{metricI.GetFloat64()}[0],
							},
						})
					default:
						// TODO: do something here
					}
				}

				for key, metrics := range countMetrics {
					encoder.Encode(&prometheusModels.MetricFamily{
						Name:   &[]string{fixPrometheusKey(prefix + k + "_" + key)}[0],
						Type:   &[]prometheusModels.MetricType{prometheusModels.MetricType_COUNTER}[0],
						Metric: metrics,
					})
				}

				for key, metrics := range gaugeMetrics {
					encoder.Encode(&prometheusModels.MetricFamily{
						Name:   &[]string{fixPrometheusKey(prefix + k + "_" + key)}[0],
						Type:   &[]prometheusModels.MetricType{prometheusModels.MetricType_GAUGE}[0],
						Metric: metrics,
					})
				}

				for key, metrics := range timingMetrics {
					encoder.Encode(&prometheusModels.MetricFamily{
						Name:   &[]string{fixPrometheusKey(prefix + k + "_" + key)}[0],
						Type:   &[]prometheusModels.MetricType{prometheusModels.MetricType_GAUGE}[0],
						Metric: metrics,
					})
				}
			default:
				// TODO: do something here
			}
		}
	}
}

func WriteMetricsPrometheus(writer io.Writer) error {
	prometheusEncoder := expfmt.NewEncoder(writer, PrometheusFormat)
	writeMetricsPrometheus(getStatus(), prometheusEncoder, ``)
	return nil
}

func WriteMetricsJSON(writer io.Writer) error {
	return json.NewEncoder(writer).Encode(getStatus())
}

func AddCustomMetricsHook(hook func() map[string]interface{}) {
	customMetricsHooks = append(customMetricsHooks, hook)
}
