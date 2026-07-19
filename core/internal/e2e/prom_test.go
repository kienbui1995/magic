//go:build e2e

package e2e

import (
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
)

// prometheusDefaultGather returns all metric families from the default
// Prometheus registry (the one promauto registers into).
func prometheusDefaultGather() ([]*dto.MetricFamily, error) {
	return prometheus.DefaultGatherer.Gather()
}
