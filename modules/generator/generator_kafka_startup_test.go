package generator

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestConfig_startupReplayHorizon verifies the horizon resolution: it defaults
// to the metrics ingestion slack (so the read horizon tracks the emit horizon),
// but an explicit startup_replay_horizon overrides it (so a large emit slack
// need not force a large startup replay).
func TestConfig_startupReplayHorizon(t *testing.T) {
	cfg := &Config{}
	f := flag.NewFlagSet("test", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults("", f)

	assert.False(t, cfg.SkipStaleBacklogOnStartup, "skip should default off")
	assert.Equal(t, cfg.MetricsIngestionSlack, cfg.startupReplayHorizon(), "unset horizon falls back to slack")

	cfg.StartupReplayHorizon = 90 * time.Second
	assert.Equal(t, 90*time.Second, cfg.startupReplayHorizon(), "explicit horizon overrides slack")
}

// TestStartupSeekOffset verifies the offset the generator resumes from when
// skip_stale_backlog_on_startup is enabled: it never rewinds behind the group's
// committed offset, but it skips forward past stale backlog to the horizon
// offset (the first record at/after now-horizon) when the commit is behind that
// horizon or absent.
func TestStartupSeekOffset(t *testing.T) {
	for _, tc := range []struct {
		name          string
		committed     int64
		horizonOffset int64
		wantOffset    int64
		wantSeek      bool
	}{
		{"committed ahead of horizon keeps position, no seek", 100, 50, 100, false},
		{"committed behind horizon skips forward", 50, 100, 100, true},
		{"no committed offset seeks to horizon", kafkaOffsetNone, 100, 100, true},
		{"committed equal to horizon does not seek", 100, 100, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offset, seek := startupSeekOffset(tc.committed, tc.horizonOffset)
			assert.Equal(t, tc.wantOffset, offset, "offset")
			assert.Equal(t, tc.wantSeek, seek, "seek")
		})
	}
}
