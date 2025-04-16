package provider

import (
	"context"
	"flag"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/tempo/modules/backendscheduler/work"
	"github.com/grafana/tempo/modules/overrides"
	"github.com/grafana/tempo/modules/storage"
	"github.com/grafana/tempo/pkg/tempopb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var redactionTracer = otel.Tracer("modules/backendscheduler/provider/redaction")

type RedactionConfig struct {
	// MeasureInterval  time.Duration           `yaml:"measure_interval"`
	// Compactor        tempodb.CompactorConfig `yaml:"compaction"`
	// MaxJobsPerTenant int                     `yaml:"max_jobs_per_tenant"`
	Backoff backoff.Config `yaml:"backoff"`
}

func (cfg *RedactionConfig) RegisterFlagsAndApplyDefaults(prefix string, f *flag.FlagSet) {
}

type RedactionProvider struct {
	cfg    RedactionConfig
	logger log.Logger

	// Dependencies needed for compaction job selection
	store     storage.Store
	overrides overrides.Interface

	// Scheduler calls required for this provider
	sched Scheduler

	// Dependencies needed for tenant selection
	// curPriority       *tenantselector.PriorityQueue
	// curTenant         *tenantselector.Item
	// curSelector       blockselector.CompactionBlockSelector
	// curTenantJobCount int
}

func NewRedactionProvider(
	cfg RedactionConfig,
	logger log.Logger,
	store storage.Store,
	overrides overrides.Interface,
	scheduler Scheduler,
) *RedactionProvider {
	return &RedactionProvider{
		cfg:       cfg,
		logger:    logger,
		store:     store,
		overrides: overrides,
		// curPriority: tenantselector.NewPriorityQueue(),
		sched: scheduler,
	}
}

func (p *RedactionProvider) Start(ctx context.Context) <-chan *work.Job {
	jobs := make(chan *work.Job, 1)

	go func() {
		defer close(jobs)

		level.Info(p.logger).Log("msg", "redaction provider started")

		var (
			job *work.Job
			b   = backoff.New(ctx, p.cfg.Backoff)
		)

		// reset := func() {
		// 	job = nil
		// 	b.Reset()
		// }

		for {
			select {
			case <-ctx.Done():
				level.Info(p.logger).Log("msg", "redaction provider stopping")
				return
			default:
				ctx, span := tracer.Start(ctx, "redaction-provider-poll")

				attributes := []attribute.KeyValue{
					// attribute.Int("max_jobs_per_tenant", p.cfg.MaxJobsPerTenant),
					// attribute.Int("jobs_in_queue", len(jobs)),
					// attribute.Int("jobs_in_scheduler", len(p.sched.ListJobs())),
				}

				span.SetAttributes(attributes...)

				if job == nil {
					// we don't have a job, get the next one
					job = p.nextRedactionJob(ctx)
					if job == nil {
						span.AddEvent("backoff")
						b.Wait()
					}
				}

				for _, tenant := range p.store.Tenants() {
					redactions := p.overrides.TraceRedactions(tenant)

					if len(redactions) > 0 {
					}

					// Check if we have already started a redaction which includes any of these.
				}

				span.End()
			}
		}
	}()

	return jobs
}

func (p *RedactionProvider) nextRedactionJob(ctx context.Context) *work.Job {
	for _, j := range p.sched.ListJobs() {
		switch j.GetType() {
		case tempopb.JobType_JOB_TYPE_REDACTION:
			switch j.GetStatus() {
			case tempopb.JobStatus_JOB_STATUS_RUNNING, tempopb.JobStatus_JOB_STATUS_UNSPECIFIED:
				return nil
			}
		}
	}

	return &work.Job{
		ID:   uuid.New().String(),
		Type: tempopb.JobType_JOB_TYPE_REDACTION,
		JobDetail: tempopb.JobDetail{
			Retention: &tempopb.RetentionDetail{},
		},
	}
}
