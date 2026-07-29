package frontend

import (
	"testing"
	"time"

	"github.com/grafana/tempo/pkg/tempopb"
	"github.com/grafana/tempo/tempodb/backend"
	"github.com/stretchr/testify/require"
)

func TestCacheKeyForJob(t *testing.T) {
	tcs := []struct {
		name          string
		tenant        string
		queryHash     uint64
		req           *tempopb.SearchRequest
		meta          *backend.BlockMeta
		searchPage    int
		pagesToSearch int

		expected string
	}{
		{
			name:      "valid!",
			tenant:    "foo",
			queryHash: 42,
			req: &tempopb.SearchRequest{
				Start: 10,
				End:   20,
			},
			meta: &backend.BlockMeta{
				BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
				StartTime: time.Unix(15, 0),
				EndTime:   time.Unix(16, 0),
			},
			searchPage:    1,
			pagesToSearch: 2,
			expected:      "sj:foo:42:00000000-0000-0000-0000-000000000123:1:2",
		},
		{
			name:      "no query hash means no query cache",
			queryHash: 0,
			req: &tempopb.SearchRequest{
				Start: 10,
				End:   20,
			},
			meta: &backend.BlockMeta{
				BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
				StartTime: time.Unix(15, 0),
				EndTime:   time.Unix(16, 0),
			},
			searchPage:    1,
			pagesToSearch: 2,
			expected:      "",
		},
		{
			name:      "meta before start time",
			queryHash: 42,
			req: &tempopb.SearchRequest{
				Start: 10,
				End:   20,
			},
			meta: &backend.BlockMeta{
				BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
				StartTime: time.Unix(5, 0),
				EndTime:   time.Unix(6, 0),
			},
			searchPage:    1,
			pagesToSearch: 2,
			expected:      "",
		},
		{
			name:      "meta overlaps search start",
			queryHash: 42,
			req: &tempopb.SearchRequest{
				Start: 10,
				End:   20,
			},
			meta: &backend.BlockMeta{
				BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
				StartTime: time.Unix(5, 0),
				EndTime:   time.Unix(15, 0),
			},
			searchPage:    1,
			pagesToSearch: 2,
			expected:      "",
		},
		{
			name:      "meta overlaps search end",
			queryHash: 42,
			req: &tempopb.SearchRequest{
				Start: 10,
				End:   20,
			},
			meta: &backend.BlockMeta{
				BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
				StartTime: time.Unix(15, 0),
				EndTime:   time.Unix(25, 0),
			},
			searchPage:    1,
			pagesToSearch: 2,
			expected:      "",
		},
		{
			name:      "meta after search range",
			queryHash: 42,
			req: &tempopb.SearchRequest{
				Start: 10,
				End:   20,
			},
			meta: &backend.BlockMeta{
				BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
				StartTime: time.Unix(25, 0),
				EndTime:   time.Unix(30, 0),
			},
			searchPage:    1,
			pagesToSearch: 2,
			expected:      "",
		},
		{
			name:      "meta encapsulates search range",
			queryHash: 42,
			req: &tempopb.SearchRequest{
				Start: 10,
				End:   20,
			},
			meta: &backend.BlockMeta{
				BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
				StartTime: time.Unix(5, 0),
				EndTime:   time.Unix(30, 0),
			},
			searchPage:    1,
			pagesToSearch: 2,
			expected:      "",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			startTime := time.Unix(int64(tc.req.Start), 0)
			endTime := time.Unix(int64(tc.req.End), 0)

			actual := searchJobCacheKey(tc.tenant, tc.queryHash, startTime, endTime, tc.meta, tc.searchPage, tc.pagesToSearch)
			require.Equal(t, tc.expected, actual)
		})
	}
}

// TestCacheKeyMetadataIgnoresTimeWindow verifies that metadata (tag / tag-value)
// sub-requests are cacheable even when the search range does not fully encapsulate
// the block, while span search (sj:) and query range (qr:) still require full
// encapsulation. A block's tag/tag-value result is independent of the search time
// range (the backend and live-store block scans return all values in the block
// regardless of start/end), so it is safe to cache a non-encapsulating metadata
// result and reuse it for any overlapping window.
func TestCacheKeyMetadataIgnoresTimeWindow(t *testing.T) {
	// window [10,20] partially overlaps the block [15,25] -> NOT encapsulating
	start := time.Unix(10, 0)
	end := time.Unix(20, 0)
	meta := &backend.BlockMeta{
		BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
		StartTime: time.Unix(15, 0),
		EndTime:   time.Unix(25, 0),
	}

	tcs := []struct {
		name      string
		prefix    string
		queryHash uint64
		expected  string
	}{
		{
			name:      "tag values cache despite non-encapsulating window",
			prefix:    cacheKeyPrefixSearchTagValues,
			queryHash: 42,
			expected:  "stv:foo:42:00000000-0000-0000-0000-000000000123:1:2",
		},
		{
			name:      "tag names cache despite non-encapsulating window",
			prefix:    cacheKeyPrefixSearchTag,
			queryHash: 42,
			expected:  "st:foo:42:00000000-0000-0000-0000-000000000123:1:2",
		},
		{
			name:      "metadata still needs a query hash",
			prefix:    cacheKeyPrefixSearchTagValues,
			queryHash: 0,
			expected:  "",
		},
		{
			name:      "span search still requires encapsulation",
			prefix:    cacheKeyPrefixSearchJob,
			queryHash: 42,
			expected:  "",
		},
		{
			name:      "query range still requires encapsulation",
			prefix:    cacheKeyPrefixQueryRange,
			queryHash: 42,
			expected:  "",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			actual := cacheKey(tc.prefix, "foo", tc.queryHash, start, end, meta, 1, 2)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func BenchmarkCacheKeyForJob(b *testing.B) {
	req := &tempopb.SearchRequest{
		Start: 10,
		End:   20,
	}
	meta := &backend.BlockMeta{
		BlockID:   backend.MustParse("00000000-0000-0000-0000-000000000123"),
		StartTime: time.Unix(15, 0),
		EndTime:   time.Unix(16, 0),
	}

	startTime := time.Unix(int64(req.Start), 0)
	endTime := time.Unix(int64(req.End), 0)

	for i := 0; i < b.N; i++ {
		s := searchJobCacheKey("foo", 10, startTime, endTime, meta, 1, 2)
		if len(s) == 0 {
			b.Fatalf("expected non-empty string")
		}
	}
}
