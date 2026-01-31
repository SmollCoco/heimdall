package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewLogEntry_PopulatesFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	labels := map[string]string{"service": "api"}
	entry := NewLogEntry([]byte("hello"), "/tmp/file.log", labels)

	assert.Equal(t, []byte("hello"), entry.Line)
	assert.Equal(t, "/tmp/file.log", entry.Source)
	assert.Equal(t, "api", entry.Labels["service"])
	assert.Equal(t, entry.Source, entry.StreamID)
	assert.False(t, entry.Timestamp.Before(now))
	assert.WithinDuration(t, time.Now(), entry.Timestamp, time.Second)
}

func TestNewLogEntry_LabelsAreCopied(t *testing.T) {
	t.Parallel()

	labels := map[string]string{"team": "platform"}
	entry := NewLogEntry([]byte("line"), "source", labels)

	labels["team"] = "mutated"
	assert.Equal(t, "platform", entry.Labels["team"])

	entry.Labels["team"] = "updated"
	assert.Equal(t, "mutated", labels["team"])
}
