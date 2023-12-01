package querier

import (
	"context"
	"fmt"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/tempo/pkg/tempopb"
	"github.com/grafana/tempo/pkg/traceql"
	"github.com/grafana/tempo/pkg/util/log"
)

func (q *Querier) QueryRange(ctx context.Context, req *tempopb.QueryRangeRequest) (*tempopb.QueryRangeResponse, error) {
	if req.Of == 0 {
		return q.queryRangeRecent(ctx, req)
	}

	// Backend requests go here
	return &tempopb.QueryRangeResponse{}, nil
}

func (q *Querier) queryRangeRecent(ctx context.Context, req *tempopb.QueryRangeRequest) (*tempopb.QueryRangeResponse, error) {
	// // Get results from all generators
	replicationSet, err := q.generatorRing.GetReplicationSetForOperation(ring.Read)
	if err != nil {
		return nil, fmt.Errorf("error finding generators in Querier.SpanMetricsSummary: %w", err)
	}
	lookupResults, err := q.forGivenGenerators(
		ctx,
		replicationSet,
		func(ctx context.Context, client tempopb.MetricsGeneratorClient) (interface{}, error) {
			return client.QueryRange(ctx, req)
		},
	)
	if err != nil {
		_ = level.Error(log.Logger).Log("error querying generators in Querier.MetricsQueryRange", "err", err)

		return nil, fmt.Errorf("error querying generators in Querier.MetricsQueryRange: %w", err)
	}

	c := traceql.QueryRangeCombiner{}
	for _, result := range lookupResults {
		c.Combine(result.response.(*tempopb.QueryRangeResponse).Series)
	}

	resp := &tempopb.QueryRangeResponse{
		Series: c.Results(),
	}

	return resp, nil
}
