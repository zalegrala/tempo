package traceql

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/grafana/tempo/pkg/tempopb"
	"github.com/grafana/tempo/pkg/util"
)

func DefaultQueryRangeStep(start time.Time, end time.Time) time.Duration {
	return time.Duration(math.Max(math.Floor(end.Sub(start).Seconds()/250), 1)) * time.Second
}

// IntervalCount is the number of intervals in the range with step.
func IntervalCount(start, end, step uint64) int {
	intervals := (end - start) / step
	intervals++
	return int(intervals)
}

// TimestampOf the given interval with the start and step.
func TimestampOf(interval, start, step uint64) uint64 {
	return start + interval*step
}

// IntervalOf the given timestamp within the range and step.
func IntervalOf(ts, start, end, step uint64) int {
	if ts < start || ts > end || end == start || step == 0 {
		// Invalid
		return -1
	}

	return int((ts - start) / step)
}

type Label struct {
	Key   Attribute
	Value Static
}

const maxGroupBys = 5 // TODO - Delete me
type LabelSet [maxGroupBys]Label

type TimeSeries struct {
	Labels LabelSet
	Values []float64
}

// TODO - Make an analog of me in tempopb proto for over-the-wire transmission
type SeriesSet map[LabelSet]TimeSeries

// VectorAggregator turns a vector of spans into a single numeric scalar
type VectorAggregator interface {
	Observe(s Span)
	Sample() float64
}

// RangeAggregator sorts spans into time slots
// TODO - for efficiency we probably combine this with VectorAggregator (see todo about CountOverTimeAggregator)
type RangeAggregator interface {
	Observe(s Span)
	Samples() []float64
}

// SpanAggregator sorts spans into series
type SpanAggregator interface {
	Observe(Span)
	Series() SeriesSet
}

// CountOverTimeAggregator counts the number of spans. It can also
// calculate the rate when given a multiplier.
// TODO - Rewrite me to be []float64 which is more efficient
type CountOverTimeAggregator struct {
	count    float64
	rateMult float64
}

var _ VectorAggregator = (*CountOverTimeAggregator)(nil)

func NewCountOverTimeAggregator() *CountOverTimeAggregator {
	return &CountOverTimeAggregator{
		rateMult: 1.0,
	}
}

func NewRateAggregator(rateMult float64) *CountOverTimeAggregator {
	return &CountOverTimeAggregator{
		rateMult: rateMult,
	}
}

func (c *CountOverTimeAggregator) Observe(_ Span) {
	c.count++
}

func (c *CountOverTimeAggregator) Sample() float64 {
	return c.count * c.rateMult
}

// StepAggregator sorts spans into time slots using a step interval like 30s or 1m
type StepAggregator struct {
	start   uint64
	end     uint64
	step    uint64
	vectors []VectorAggregator
}

var _ RangeAggregator = (*StepAggregator)(nil)

func NewStepAggregator(start, end, step uint64, innerAgg func() VectorAggregator) *StepAggregator {
	intervals := IntervalCount(start, end, step)
	vectors := make([]VectorAggregator, intervals)
	for i := range vectors {
		vectors[i] = innerAgg()
	}

	return &StepAggregator{
		start:   start,
		end:     end,
		step:    step,
		vectors: vectors,
	}
}

func (s *StepAggregator) Observe(span Span) {
	st := span.StartTimeUnixNanos()
	if st < s.start || st >= s.end {
		// Out of bounds, maybe this needs to be checked higher up
		return
	}
	interval := (st - s.start) / s.step
	s.vectors[interval].Observe(span)
}

func (s *StepAggregator) Samples() []float64 {
	ss := make([]float64, len(s.vectors))
	for i, v := range s.vectors {
		ss[i] = v.Sample()
	}
	return ss
}

type GroupingAggregator struct {
	series    map[LabelSet]RangeAggregator
	by        []Attribute   // Original attributes: .foo
	byLookups [][]Attribute // Lookups: span.foo resource.foo
	innerAgg  func() RangeAggregator
}

var _ SpanAggregator = (*GroupingAggregator)(nil)

func NewGroupingAggregator(innerAgg func() RangeAggregator, by []Attribute) SpanAggregator {
	if len(by) == 0 {
		return &UngroupedAggregator{
			innerAgg: innerAgg(),
		}
	}

	lookups := make([][]Attribute, len(by))
	for i, attr := range by {
		if attr.Intrinsic == IntrinsicNone && attr.Scope == AttributeScopeNone {
			// Unscoped attribute. Also check span-level, then resource-level.
			lookups[i] = []Attribute{
				attr,
				NewScopedAttribute(AttributeScopeSpan, false, attr.Name),
				NewScopedAttribute(AttributeScopeResource, false, attr.Name),
			}
		} else {
			lookups[i] = []Attribute{attr}
		}
	}

	return &GroupingAggregator{
		series:    map[LabelSet]RangeAggregator{},
		by:        by,
		byLookups: lookups,
		innerAgg:  innerAgg,
	}
}

func (g *GroupingAggregator) Observe(span Span) {
	labels := LabelSet{}
	for i, k := range g.by {
		labels[i].Key = k
		labels[i].Value = lookup(g.byLookups[i], span.Attributes())
	}

	series, ok := g.series[labels]
	if !ok {
		series = g.innerAgg()
		g.series[labels] = series
	}

	series.Observe(span)
}

func (g *GroupingAggregator) Series() SeriesSet {
	ss := SeriesSet{}

	for k, v := range g.series {
		ss[k] = TimeSeries{
			Labels: k,
			Values: v.Samples(),
		}
	}

	return ss
}

// UngroupedAggregator builds a single series with no labels. e.g. {} | rate()
type UngroupedAggregator struct {
	innerAgg RangeAggregator
}

var _ SpanAggregator = (*UngroupedAggregator)(nil)

func (u *UngroupedAggregator) Observe(span Span) {
	u.innerAgg.Observe(span)
}

func (u *UngroupedAggregator) Series() SeriesSet {
	l := LabelSet{}
	return SeriesSet{
		l: {
			Labels: l,
			Values: u.innerAgg.Samples(),
		},
	}
}

// ExecuteMetricsQueryRange - Execute the given metrics query. Just a wrapper around CompileMetricsQueryRange
func (e *Engine) ExecuteMetricsQueryRange(ctx context.Context, req *tempopb.QueryRangeRequest, fetcher SpansetFetcher) (results SeriesSet, err error) {
	eval, err := e.CompileMetricsQueryRange(req)
	if err != nil {
		return nil, err
	}

	err = eval.Do(ctx, fetcher)
	if err != nil {
		return nil, err
	}

	return eval.Results()
}

// CompileMetricsQueryRange returns an evalulator that can be reused across multiple data sources.
func (e *Engine) CompileMetricsQueryRange(req *tempopb.QueryRangeRequest) (*MetricsEvalulator, error) {
	if req.Start <= 0 {
		return nil, fmt.Errorf("start required")
	}
	if req.End <= 0 {
		return nil, fmt.Errorf("end required")
	}
	if req.End <= req.Start {
		return nil, fmt.Errorf("end must be greater than start")
	}
	if req.Step <= 0 {
		return nil, fmt.Errorf("step required")
	}

	eval, metricsPipeline, storageReq, err := e.Compile(req.Query)
	if err != nil {
		return nil, fmt.Errorf("compiling query: %w", err)
	}

	if metricsPipeline == nil {
		return nil, fmt.Errorf("not a metrics query")
	}

	// This initializes all step buffers, counters, etc
	metricsPipeline.init(req)

	me := &MetricsEvalulator{
		storageReq:      storageReq,
		metricsPipeline: metricsPipeline,
	}

	if req.Of > 1 {
		// Trace id sharding
		// Select traceID if not already present.  It must be in the first pass
		// so that we only evalulate our traces.
		storageReq.Shard = int(req.Shard)
		storageReq.Of = int(req.Of)
		traceID := NewIntrinsic(IntrinsicTraceID)
		if !storageReq.HasAttribute(traceID) {
			storageReq.Conditions = append(storageReq.Conditions, Condition{Attribute: traceID})
		}
	}

	// Sharding algorithm
	// In order to scale out queries like {A} >> {B} | rate()  we need specific
	// rules about the data is divided across time boundary shards.  These
	// spans can cross hours or days and the simple idea to just check span
	// start time won't work.
	// Therefore results are matched with the following rules:
	// (1) Evalulate the query for any overlapping trace
	// (2) For any matching spans: only include the ones that started in this time frame.
	// This will increase redundant trace evalulation, but it ensures that matching spans are
	// gauranteed to be found and included in the metrcs.
	startTime := NewIntrinsic(IntrinsicSpanStartTime)
	if !storageReq.HasAttribute(startTime) {
		if storageReq.AllConditions {
			// The most efficient case.  We can add it to the primary pass
			// without affecting correctness. And this lets us avoid the
			// entire second pass.
			storageReq.Conditions = append(storageReq.Conditions, Condition{Attribute: startTime})
		} else {
			// Complex query with a second pass. In this case it is better to
			// add it to the second pass so that it's only returned for the matches.
			storageReq.SecondPassConditions = append(storageReq.SecondPassConditions, Condition{Attribute: startTime})
		}
	}
	// (1) any overlapping trace
	storageReq.StartTimeUnixNanos = req.Start
	storageReq.EndTimeUnixNanos = req.End // Should this be exclusive?
	// (2) Only include spans that started in this time frame.
	//     This is checked inside the evaluator
	me.checkTime = true
	me.start = req.Start
	me.end = req.End

	// Avoid a second pass when not needed for much better performance.
	// TODO - Is there any case where eval() needs to be called but AllConditions=true?
	if !storageReq.AllConditions || len(storageReq.SecondPassConditions) > 0 {
		storageReq.SecondPass = func(s *Spanset) ([]*Spanset, error) {
			return eval([]*Spanset{s})
		}
	}

	return me, nil
}

func lookup(needles []Attribute, haystack map[Attribute]Static) Static {
	for _, n := range needles {
		if v, ok := haystack[n]; ok {
			return v
		}
	}

	return Static{}
}

type MetricsEvalulator struct {
	start, end      uint64
	checkTime       bool
	storageReq      *FetchSpansRequest
	metricsPipeline metricsFirstStageElement
}

func (e *MetricsEvalulator) Do(ctx context.Context, f SpansetFetcher) error {
	fetch, err := f.Fetch(ctx, *e.storageReq)
	if errors.Is(err, util.ErrUnsupported) {
		return nil
	}
	if err != nil {
		return err
	}

	defer fetch.Results.Close()

	for {
		ss, err := fetch.Results.Next(ctx)
		if err != nil {
			return nil
		}
		if ss == nil {
			break
		}

		for _, s := range ss.Spans {
			if e.checkTime {
				st := s.StartTimeUnixNanos()
				if st < e.start || st >= e.end {
					continue
				}
			}

			e.metricsPipeline.observe(s)
		}

		ss.Release()
	}

	return nil
}

func (e *MetricsEvalulator) Results() (SeriesSet, error) {
	return e.metricsPipeline.result(), nil
}
