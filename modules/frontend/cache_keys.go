package frontend

import (
	"strconv"
	"strings"
	"time"

	"github.com/grafana/tempo/tempodb/backend"
)

const (
	cacheKeyPrefixSearchJob       = "sj:"
	cacheKeyPrefixSearchTag       = "st:"
	cacheKeyPrefixSearchTagValues = "stv:"
	cacheKeyPrefixQueryRange      = "qr:"
)

func searchJobCacheKey(tenant string, queryHash uint64, start, end time.Time, meta *backend.BlockMeta, startPage, pagesToSearch int) string {
	return cacheKey(cacheKeyPrefixSearchJob, tenant, queryHash, start, end, meta, startPage, pagesToSearch)
}

func queryRangeCacheKey(tenant string, queryHash uint64, start, end time.Time, meta *backend.BlockMeta, startPage, pagesToSearch int) string {
	return cacheKey(cacheKeyPrefixQueryRange, tenant, queryHash, start, end, meta, startPage, pagesToSearch)
}

// cacheKey returns a string that can be used as a cache key for a backend search job. if a valid key cannot be calculated
// it returns an empty string.
func cacheKey(prefix string, tenant string, queryHash uint64, start, end time.Time, meta *backend.BlockMeta, startPage, pagesToSearch int) string {
	// if the query hash is 0 we can't cache. this may occur if the user is using the old search api
	if queryHash == 0 {
		return ""
	}

	// metadata (tag / tag-value) results for a block are independent of the search time range: the
	// backend and live-store block scans return every value present in the block regardless of
	// start/end (only block selection uses the time range, not the per-block scan). So a metadata
	// result computed for one window is valid for any other window over the same block, and we can
	// cache it even when the search range does not encapsulate the block. This matches the live-store
	// tag-values disk cache, whose key also omits start/end.
	isMetadata := prefix == cacheKeyPrefixSearchTag || prefix == cacheKeyPrefixSearchTagValues

	// for span search and query range the per-block result depends on the search range, so unless the
	// search range completely encapsulates the block range we can't cache. this is b/c different search
	// ranges will return different results for a given block unless the search range covers the entire block
	if !isMetadata &&
		!(start.Before(meta.StartTime) && // search start is before block start
			end.After(meta.EndTime)) { // search end is after block end
		return ""
	}

	sb := strings.Builder{}
	sb.Grow(len(prefix) +
		len(tenant) +
		1 + // :
		20 + // query hash
		1 + // :
		36 + // block id
		1 + // :
		3 + // start page
		1 + // :
		2) // 2 for pages to search
	sb.WriteString(prefix)
	sb.WriteString(tenant)
	sb.WriteString(":")
	sb.WriteString(strconv.FormatUint(queryHash, 10))
	sb.WriteString(":")
	sb.WriteString(meta.BlockID.String())
	sb.WriteString(":")
	sb.WriteString(strconv.Itoa(startPage))
	sb.WriteString(":")
	sb.WriteString(strconv.Itoa(pagesToSearch))

	return sb.String()
}
