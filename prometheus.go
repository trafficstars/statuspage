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

	"github.com/trafficstars/fastmetrics"
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
			case []*metrics.Metric:
				countMetrics := map[string][]*prometheusModels.Metric{}
				gaugeMetrics := map[string][]*prometheusModels.Metric{}
				timingMetrics := map[string][]*prometheusModels.Metric{}

				for _, metric := range v {
					workerI := metric.GetWorker()
					key := metric.GetName()
					var labels []*prometheusModels.LabelPair
					for k, v := range metric.GetTags() {
						value := metrics.TagValueToString(v)
						if !utf8.ValidString(value) {
							value = base64.StdEncoding.EncodeToString([]byte(value))
						}
						labels = append(labels, &prometheusModels.LabelPair{
							Name:  &[]string{k}[0],
							Value: &[]string{value}[0],
						})
					}
					switch workerI.GetType() {
					case metrics.MetricTypeTiming:
						worker := workerI.(metrics.WorkerTiming)
						values := worker.GetValuePointers()

						addTimingMetric := func(key string, v uint64) {
							timingMetrics[key] = append(timingMetrics[key], &prometheusModels.Metric{
								Label: labels,
								Gauge: &prometheusModels.Gauge{
									Value: &[]float64{float64(v)}[0],
								},
							})
						}

						considerValue := func(label string) func(data *metrics.TimingValue) {
							return func(data *metrics.TimingValue) {
								if data.Count == 0 {
									return
								}
								addTimingMetric(key+`_`+label+`_count`, data.Count)
								addTimingMetric(key+`_`+label+`_min`, data.Min)
								addTimingMetric(key+`_`+label+`_mid`, data.Mid)
								addTimingMetric(key+`_`+label+`_avg`, data.Avg)
								addTimingMetric(key+`_`+label+`_per99`, data.Per99)
								addTimingMetric(key+`_`+label+`_max`, data.Max)
							}
						}

						values.Last.LockDo(considerValue(`last`))
						values.S1.LockDo(considerValue(`1s`))
						values.S5.LockDo(considerValue(`5s`))
						values.M1.LockDo(considerValue(`1m`))
						values.M5.LockDo(considerValue(`5m`))
						values.H1.LockDo(considerValue(`1h`))
						values.H6.LockDo(considerValue(`6h`))
						values.D1.LockDo(considerValue(`1d`))
						values.Total.LockDo(considerValue(`total`))
					case metrics.MetricTypeCount:
						countMetrics[key] = append(countMetrics[key], &prometheusModels.Metric{
							Label: labels,
							Counter: &prometheusModels.Counter{
								Value: &[]float64{float64(workerI.Get())}[0],
							},
						})
					case metrics.MetricTypeGauge:
						var value float64
						if getFloater, ok := workerI.(interface{ GetFloat() float64 }); ok {
							value = getFloater.GetFloat()
						} else {
							value = float64(workerI.Get())
						}
						gaugeMetrics[key] = append(gaugeMetrics[key], &prometheusModels.Metric{
							Label: labels,
							Gauge: &prometheusModels.Gauge{
								Value: &value,
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
