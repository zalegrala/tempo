package traceql

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/tempo/pkg/util"
)

const maxGroupBys = 5 // TODO - Delete me

// TODO - Move me to tempopb proto
type MetricsQueryRangeRequest struct {
	Q          string
	Start, End uint64 // Time window in unix nanoseconds. Start inclusive, end exclusive
	Step       uint64 // Step duration in nanoseconds (30s, 1m, 1h)
}

type Label struct {
	Key   Attribute
	Value Static
}

type LabelSet [5]Label

type TimeSeries struct {
	Labels LabelSet
	Values []float64
}

// TODO - Move me to tempopb proto
type SeriesSet struct {
	Series []TimeSeries
}

type VectorAggregator interface {
	Observe(s Span)
	Sample() float64
}

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

type RangeAggregator interface {
	Observe(s Span)
	Samples() []float64
}

type StepAggregator struct {
	start   uint64
	end     uint64
	step    uint64
	vectors []VectorAggregator
}

var _ RangeAggregator = (*StepAggregator)(nil)

func NewStepAggregator(start, end, step uint64, innerAgg func() VectorAggregator) *StepAggregator {
	intervals := (end - start) / step
	vectors := make([]VectorAggregator, intervals+1)
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

type SpanAggregator interface {
	Observe(Span)
	Series() SeriesSet
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
	ss := make([]TimeSeries, 0, len(g.series))

	for k, v := range g.series {
		ts := TimeSeries{
			Labels: k,
			Values: v.Samples(),
		}
		ss = append(ss, ts)
	}

	return SeriesSet{
		Series: ss,
	}
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
	return SeriesSet{
		Series: []TimeSeries{{
			Labels: LabelSet{},
			Values: u.innerAgg.Samples(),
		}},
	}
}

func (e *Engine) MetricsQueryRange(ctx context.Context, req MetricsQueryRangeRequest, fetcher SpansetFetcher) (results SeriesSet, err error) {
	if req.Start <= 0 {
		return SeriesSet{}, fmt.Errorf("start required")
	}
	if req.End <= 0 {
		return SeriesSet{}, fmt.Errorf("end required")
	}
	if req.End <= req.Start {
		return SeriesSet{}, fmt.Errorf("end must be greater than start")
	}
	if req.Step <= 0 {
		return SeriesSet{}, fmt.Errorf("step required")
	}

	// TODO - This needs to validate the non-metrics pipeline too
	eval, metricsPipeline, storageReq, err := e.Compile(req.Q)
	if err != nil {
		return SeriesSet{}, fmt.Errorf("compiling query: %w", err)
	}

	err = metricsPipeline.validate()
	if err != nil {
		return SeriesSet{}, err
	}

	var (
		startTime  = NewIntrinsic(IntrinsicSpanStartTime)
		startValue = NewStaticInt(int(req.Start))
		endValue   = NewStaticInt(int(req.End))
	)

	storageReq.StartTimeUnixNanos = req.Start
	storageReq.EndTimeUnixNanos = req.End
	storageReq.Conditions = append(storageReq.Conditions, Condition{Attribute: startTime, Op: OpGreaterEqual, Operands: []Static{startValue}}) // move this to ast extractConditions?
	storageReq.Conditions = append(storageReq.Conditions, Condition{Attribute: startTime, Op: OpLess, Operands: []Static{endValue}})

	// We don't need a second pass for some cases - i think??
	if !storageReq.AllConditions || len(storageReq.SecondPassConditions) > 0 {
		storageReq.SecondPass = func(s *Spanset) ([]*Spanset, error) {
			return eval([]*Spanset{s})
		}
	}

	fetch, err := fetcher.Fetch(ctx, *storageReq)
	if errors.Is(err, util.ErrUnsupported) {
		return SeriesSet{}, nil
	}
	if err != nil {
		return SeriesSet{}, err
	}

	defer fetch.Results.Close()

	// This initializes all step buffers, counters, etc
	metricsPipeline.init(req)

	for {
		ss, err := fetch.Results.Next(ctx)
		if err != nil {
			return SeriesSet{}, err
		}
		if ss == nil {
			break
		}

		for _, s := range ss.Spans {
			metricsPipeline.observe(s)
		}

		ss.Release()
	}

	// fmt.Println("Bytes read:", humanize.Bytes(fetch.Bytes()))

	return metricsPipeline.result(), nil
}

func lookup(needles []Attribute, haystack map[Attribute]Static) Static {
	for _, n := range needles {
		if v, ok := haystack[n]; ok {
			return v
		}
	}

	return Static{}
}
