package repository

import (
	"strings"
	"testing"
	"time"

	// Third-party imports
	"github.com/stretchr/testify/require"
)

func TestNewPersistenceManager_UnsupportedStrategy(t *testing.T) {
	pm, err := NewPersistenceManager(&Config{Strategy: "nope"})
	require.Error(t, err)
	require.Nil(t, pm)
	require.Contains(t, err.Error(), "unsupported persistence repository")
}

func TestNewPersistenceManager_NotImplementedStrategies(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		wantMsg  string
	}{
		{name: "dynamodb", strategy: "dynamodb", wantMsg: "not yet implemented"},
		{name: "redis", strategy: "redis", wantMsg: "not yet implemented"},
		{name: "memory", strategy: "memory", wantMsg: "not yet implemented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPersistenceManager(&Config{Strategy: tt.strategy})
			require.Error(t, err)
			require.Nil(t, pm)
			require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantMsg))
		})
	}
}

func TestGenerateHistoryID_UniqueForDifferentTimestamps(t *testing.T) {
	t1 := time.Unix(100, 1)
	t2 := time.Unix(100, 2)

	id1 := generateHistoryID("exec-x", "StateY", t1)
	id2 := generateHistoryID("exec-x", "StateY", t2)

	require.NotEqual(t, id1, id2)
	require.True(t, strings.HasPrefix(id1, "exec-x-StateY-"))
	require.True(t, strings.HasPrefix(id2, "exec-x-StateY-"))
}
