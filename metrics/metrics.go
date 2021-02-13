package metrics

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type namesub struct {
	namespace, subsystem string
}

var (
	registeredNS = sync.Map{}

	ErrNSCollision = errors.New("namespace and subsystem combination is already registered")
)

type Boxer interface {
	Count() int
	OnFlyCounts() map[string]int32
}

type metrics struct {
	m                sync.RWMutex
	jobsCountMetrics prometheus.Gauge
	onFlyJobs        *prometheus.HistogramVec
}

func New(boxer Boxer, namespace, subSystem string) error {
	if _, ok := registeredNS.Load(namesub{namespace, subSystem}); ok {
		return ErrNSCollision
	}
	registeredNS.Store(namesub{namespace, subSystem}, true)
	ms := metrics{
		m: sync.RWMutex{},
		jobsCountMetrics: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subSystem,
				Name:        "jobs_count",
				Help:        "Jobs Count",
				ConstLabels: nil,
			},
		),
		onFlyJobs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:   namespace,
			Subsystem:   subSystem,
			Name:        "on_fly_jobs",
			Help:        "On Fly Jobs",
			ConstLabels: nil,
			Buckets:     nil,
		}, []string{"host", "queue"}),
	}

	prometheus.MustRegister(ms.jobsCountMetrics)
	prometheus.MustRegister(ms.onFlyJobs)

	ms.collectMetrics(boxer)
	return nil
}

func (ms *metrics) collectMetrics(boxer Boxer) {
	go func(b Boxer) {
		for {
			ms.m.RLock()
			ms.jobsCountMetrics.Set(float64(b.Count()))

			hostname, err := os.Hostname()
			for queue, count := range b.OnFlyCounts() {
				var labels []string
				if err == nil {
					labels = append(labels, hostname)
				}

				labels = append(labels, queue)

				ms.onFlyJobs.WithLabelValues(labels...).Observe(float64(count))
			}
			ms.m.RUnlock()
			time.Sleep(10 * time.Second)
		}
	}(boxer)
}
