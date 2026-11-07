//go:build integration
// +build integration

package repository

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — ExecutionFilter extensions (raw Postgres)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *PostgresIntegrationTestSuite) TestExecutionFilter_ExecutionID() {
	baseTime := time.Now().Add(-1 * time.Hour)

	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-pg-eid-%03d", i),
			StateMachineID: "sm-pg-eid",
			Name:           fmt.Sprintf("pg-eid-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		ExecutionID: "exec-pg-eid-002",
	})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), results, 1)
	assert.Equal(suite.T(), "exec-pg-eid-002", results[0].ExecutionID)

	count, err := suite.repository.CountExecutions(suite.ctx, &ExecutionFilter{
		ExecutionID: "exec-pg-eid-002",
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count)
}

func (suite *PostgresIntegrationTestSuite) TestExecutionFilter_StateName_StateStatus() {
	baseTime := time.Now().Add(-2 * time.Hour)

	execStartTimes := make(map[int]time.Time)
	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		execStartTimes[i] = st
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-pg-sn-%03d", i),
			StateMachineID: "sm-pg-sn",
			Name:           fmt.Sprintf("pg-sn-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	// exec-pg-sn-001: ProcessState SUCCEEDED
	st1 := execStartTimes[1]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-pg-sn-1", "exec-pg-sn-001", "ProcessState", &st1, 1)))

	// exec-pg-sn-002: ProcessState FAILED
	st2 := execStartTimes[2]
	sh2 := makeStateHistory("sh-pg-sn-2", "exec-pg-sn-002", "ProcessState", &st2, 1)
	sh2.Status = "FAILED"
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh2))

	// exec-pg-sn-003: DifferentState SUCCEEDED
	st3 := execStartTimes[3]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-pg-sn-3", "exec-pg-sn-003", "DifferentState", &st3, 1)))

	suite.Run("FilterByStateName", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sn",
			StateName:      "ProcessState",
		})
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), results, 2)
		ids := collectExecutionIDs(results)
		assert.Contains(suite.T(), ids, "exec-pg-sn-001")
		assert.Contains(suite.T(), ids, "exec-pg-sn-002")
	})

	suite.Run("FilterByStateNameAndStatus", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sn",
			StateName:      "ProcessState",
			StateStatus:    "SUCCEEDED",
		})
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), results, 1)
		assert.Equal(suite.T(), "exec-pg-sn-001", results[0].ExecutionID)
	})

	suite.Run("FilterByStateStatusOnly", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sn",
			StateStatus:    "FAILED",
		})
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), results, 1)
		assert.Equal(suite.T(), "exec-pg-sn-002", results[0].ExecutionID)
	})

	suite.Run("CountMatchesListFilter", func() {
		count, err := suite.repository.CountExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sn",
			StateName:      "ProcessState",
		})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(2), count)
	})
}

func (suite *PostgresIntegrationTestSuite) TestExecutionFilter_SortByOrder() {
	baseTime := time.Now().Add(-3 * time.Hour)

	statuses := []string{"FAILED", "RUNNING", "SUCCEEDED"}
	for i, status := range statuses {
		st := baseTime.Add(time.Duration(i*10) * time.Minute)
		endT := st.Add(time.Duration((3-i)*5) * time.Minute)
		rec := &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-pg-sort-%03d", i+1),
			StateMachineID: "sm-pg-sort",
			Name:           fmt.Sprintf("pg-sort-test-%d", i+1),
			Status:         status,
			StartTime:      &st,
			CurrentState:   "Final",
		}
		if status != "RUNNING" {
			rec.EndTime = &endT
		}
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, rec))
	}

	suite.Run("SortByStartTimeDesc", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sort",
			SortBy:         "startTime",
			Order:          "desc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), results, 3)
		assert.Equal(suite.T(), "exec-pg-sort-003", results[0].ExecutionID)
		assert.Equal(suite.T(), "exec-pg-sort-001", results[2].ExecutionID)
	})

	suite.Run("SortByStartTimeAsc", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sort",
			SortBy:         "startTime",
			Order:          "asc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), results, 3)
		assert.Equal(suite.T(), "exec-pg-sort-001", results[0].ExecutionID)
		assert.Equal(suite.T(), "exec-pg-sort-003", results[2].ExecutionID)
	})

	suite.Run("SortByStatus", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sort",
			SortBy:         "status",
			Order:          "asc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), results, 3)
		assert.Equal(suite.T(), "FAILED", results[0].Status)
		assert.Equal(suite.T(), "RUNNING", results[1].Status)
		assert.Equal(suite.T(), "SUCCEEDED", results[2].Status)
	})

	suite.Run("DefaultSortBehavior", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-pg-sort",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), results, 3)
		assert.Equal(suite.T(), "exec-pg-sort-003", results[0].ExecutionID)
	})
}

func (suite *PostgresIntegrationTestSuite) TestExecutionFilter_StartBefore() {
	baseTime := time.Now().Add(-2 * time.Hour)

	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i*20) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-pg-sb-%03d", i),
			StateMachineID: "sm-pg-sb",
			Name:           fmt.Sprintf("pg-sb-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-pg-sb",
		StartBefore:    baseTime.Add(45 * time.Minute),
	})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), results, 2)
	ids := collectExecutionIDs(results)
	assert.Contains(suite.T(), ids, "exec-pg-sb-001")
	assert.Contains(suite.T(), ids, "exec-pg-sb-002")
	assert.NotContains(suite.T(), ids, "exec-pg-sb-003")
}

func (suite *PostgresIntegrationTestSuite) TestExecutionFilter_StartAfterAndStartBefore_Combined() {
	baseTime := time.Now().Add(-3 * time.Hour)

	for i := 1; i <= 5; i++ {
		st := baseTime.Add(time.Duration(i*10) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-pg-range-%03d", i),
			StateMachineID: "sm-pg-range",
			Name:           fmt.Sprintf("pg-range-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-pg-range",
		StartAfter:     baseTime.Add(15 * time.Minute),
		StartBefore:    baseTime.Add(35 * time.Minute),
	})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), results, 2)
	ids := collectExecutionIDs(results)
	assert.Contains(suite.T(), ids, "exec-pg-range-002")
	assert.Contains(suite.T(), ids, "exec-pg-range-003")
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — LinkedExecutionFilter sort/order (raw Postgres)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *PostgresIntegrationTestSuite) TestLinkedExecutionFilter_SortByOrder() {
	baseTime := time.Now().Add(-1 * time.Hour)

	smIDs := []string{"sm-pg-b", "sm-pg-a", "sm-pg-c"}
	targetNames := []string{"TargetZ", "TargetA", "TargetM"}

	for i := 0; i < 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-pg-le-sort-%03d", i+1),
			StateMachineID: smIDs[i],
			Name:           fmt.Sprintf("pg-le-sort-%d", i+1),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))

		require.NoError(suite.T(), suite.repository.SaveLinkedExecution(suite.ctx, &LinkedExecutionRecord{
			ID:                     fmt.Sprintf("link-pg-le-sort-%03d", i+1),
			SourceStateMachineID:   smIDs[i],
			SourceExecutionID:      fmt.Sprintf("exec-pg-le-sort-%03d", i+1),
			TargetStateMachineName: targetNames[i],
			TargetExecutionID:      fmt.Sprintf("exec-pg-le-sort-target-%03d", i+1),
			CreatedAt:              baseTime.Add(time.Duration(i*10) * time.Minute),
		}))
	}

	suite.Run("SortByCreatedAtAsc", func() {
		links, err := suite.repository.ListLinkedExecutions(suite.ctx, &LinkedExecutionFilter{
			SortBy: "createdAt",
			Order:  "asc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), links, 3)
		assert.Equal(suite.T(), "link-pg-le-sort-001", links[0].ID)
		assert.Equal(suite.T(), "link-pg-le-sort-003", links[2].ID)
	})

	suite.Run("SortBySourceStateMachineIdAsc", func() {
		links, err := suite.repository.ListLinkedExecutions(suite.ctx, &LinkedExecutionFilter{
			SortBy: "sourceStateMachineId",
			Order:  "asc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), links, 3)
		assert.Equal(suite.T(), "sm-pg-a", links[0].SourceStateMachineID)
		assert.Equal(suite.T(), "sm-pg-c", links[2].SourceStateMachineID)
	})

	suite.Run("SortByTargetStateMachineNameDesc", func() {
		links, err := suite.repository.ListLinkedExecutions(suite.ctx, &LinkedExecutionFilter{
			SortBy: "targetStateMachineName",
			Order:  "desc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), links, 3)
		assert.Equal(suite.T(), "TargetZ", links[0].TargetStateMachineName)
		assert.Equal(suite.T(), "TargetA", links[2].TargetStateMachineName)
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — ListStateHistory (cursor pagination, raw Postgres)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *PostgresIntegrationTestSuite) TestListStateHistory() {
	baseTime := time.Now().Add(-2 * time.Hour)

	exec := &ExecutionRecord{
		ExecutionID:    "exec-pg-lsh-001",
		StateMachineID: "sm-pg-lsh",
		Name:           "pg-lsh-test",
		Status:         "SUCCEEDED",
		StartTime:      &baseTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, exec))

	for i := 0; i < 5; i++ {
		sh := makeStateHistory(fmt.Sprintf("sh-pg-lsh-%d", i), "exec-pg-lsh-001", fmt.Sprintf("State%d", i), &baseTime, i)
		require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh))
	}

	suite.Run("AllRecords_NoLimit", func() {
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-pg-lsh-001", nil, 0, "")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 5)
		assert.Equal(suite.T(), 0, records[0].SequenceNumber)
		assert.Equal(suite.T(), 4, records[4].SequenceNumber)
	})

	suite.Run("FirstPage", func() {
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-pg-lsh-001", nil, 2, "asc")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 2)
		assert.Equal(suite.T(), 0, records[0].SequenceNumber)
		assert.Equal(suite.T(), 1, records[1].SequenceNumber)
	})

	suite.Run("CursorPage", func() {
		afterSeq := 1
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-pg-lsh-001", &afterSeq, 2, "asc")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 2)
		assert.Equal(suite.T(), 2, records[0].SequenceNumber)
		assert.Equal(suite.T(), 3, records[1].SequenceNumber)
	})

	suite.Run("CursorPage_LastPage", func() {
		afterSeq := 3
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-pg-lsh-001", &afterSeq, 10, "asc")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 1)
		assert.Equal(suite.T(), 4, records[0].SequenceNumber)
	})

	suite.Run("DescOrder", func() {
		records, _, err := suite.repository.ListStateHistory(suite.ctx, "exec-pg-lsh-001", nil, 3, "desc")
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), records, 3)
		assert.Equal(suite.T(), 4, records[0].SequenceNumber)
		assert.Equal(suite.T(), 3, records[1].SequenceNumber)
		assert.Equal(suite.T(), 2, records[2].SequenceNumber)
	})

	suite.Run("EmptyExecution", func() {
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-pg-nonexistent", nil, 0, "")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(0), total)
		assert.Len(suite.T(), records, 0)
	})

	suite.Run("EmptyExecutionID_Error", func() {
		_, _, err := suite.repository.ListStateHistory(suite.ctx, "", nil, 0, "")
		require.Error(suite.T(), err)
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — GetDashboardStats (raw Postgres)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *PostgresIntegrationTestSuite) TestGetDashboardStats() {
	suite.Run("EmptyDatabase", func() {
		stats, err := suite.repository.GetDashboardStats(suite.ctx)
		require.NoError(suite.T(), err)
		require.NotNil(suite.T(), stats)
		assert.Equal(suite.T(), int64(0), stats.TotalExecutions)
		assert.Equal(suite.T(), int64(0), stats.TotalStateMachines)
		assert.Nil(suite.T(), stats.DurationStats)
		assert.Empty(suite.T(), stats.StatusCounts)
		assert.Empty(suite.T(), stats.RecentFailures)
		assert.False(suite.T(), stats.Timestamp.IsZero())
	})

	suite.Run("WithData", func() {
		suite.cleanupTestData()

		for i := 1; i <= 2; i++ {
			require.NoError(suite.T(), suite.repository.SaveStateMachine(suite.ctx, &StateMachineRecord{
				ID:         fmt.Sprintf("sm-pg-dash-%d", i),
				Name:       fmt.Sprintf("pg-dash-sm-%d", i),
				Definition: "{}",
				Version:    "1.0",
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}))
		}

		now := time.Now()

		for i := 1; i <= 3; i++ {
			st := now.Add(time.Duration(-i) * time.Hour)
			et := st.Add(30 * time.Second)
			require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
				ExecutionID:    fmt.Sprintf("exec-pg-dash-succ-%d", i),
				StateMachineID: "sm-pg-dash-1",
				Name:           fmt.Sprintf("pg-dash-succ-%d", i),
				Status:         "SUCCEEDED",
				StartTime:      &st,
				EndTime:        &et,
				CurrentState:   "Final",
			}))
		}

		for i := 1; i <= 2; i++ {
			st := now.Add(time.Duration(-i*2) * time.Hour)
			require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
				ExecutionID:    fmt.Sprintf("exec-pg-dash-fail-%d", i),
				StateMachineID: "sm-pg-dash-1",
				Name:           fmt.Sprintf("pg-dash-fail-%d", i),
				Status:         "FAILED",
				StartTime:      &st,
				CurrentState:   "ErrorState",
				Error:          fmt.Sprintf("error-%d", i),
			}))
		}

		st := now.Add(-30 * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    "exec-pg-dash-run-1",
			StateMachineID: "sm-pg-dash-2",
			Name:           "pg-dash-run-1",
			Status:         "RUNNING",
			StartTime:      &st,
			CurrentState:   "Processing",
		}))

		stats, err := suite.repository.GetDashboardStats(suite.ctx)
		require.NoError(suite.T(), err)
		require.NotNil(suite.T(), stats)

		assert.Equal(suite.T(), int64(2), stats.TotalStateMachines)
		assert.Equal(suite.T(), int64(6), stats.TotalExecutions)

		statusMap := make(map[string]int64)
		for _, sc := range stats.StatusCounts {
			statusMap[sc.Status] = sc.Count
		}
		assert.Equal(suite.T(), int64(3), statusMap["SUCCEEDED"])
		assert.Equal(suite.T(), int64(2), statusMap["FAILED"])
		assert.Equal(suite.T(), int64(1), statusMap["RUNNING"])

		require.NotNil(suite.T(), stats.DurationStats)
		require.NotNil(suite.T(), stats.DurationStats.AvgDuration)
		assert.InDelta(suite.T(), 30.0, *stats.DurationStats.AvgDuration, 1.0)

		assert.Len(suite.T(), stats.RecentFailures, 2)
		assert.Equal(suite.T(), "FAILED", stats.RecentFailures[0].Status)

		assert.False(suite.T(), stats.Timestamp.IsZero())
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — GetDistinctStateStatuses (raw Postgres)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *PostgresIntegrationTestSuite) TestGetDistinctStateStatuses() {
	baseTime := time.Now().Add(-2 * time.Hour)

	execStartTimes := make(map[int]time.Time)
	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		execStartTimes[i] = st
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-pg-dss-%03d", i),
			StateMachineID: "sm-pg-dss-1",
			Name:           fmt.Sprintf("pg-dss-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	st1 := execStartTimes[1]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-pg-dss-1a", "exec-pg-dss-001", "ProcessState", &st1, 1)))
	sh1b := makeStateHistory("sh-pg-dss-1b", "exec-pg-dss-001", "ValidateState", &st1, 2)
	sh1b.Status = "FAILED"
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh1b))

	st2 := execStartTimes[2]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-pg-dss-2", "exec-pg-dss-002", "ProcessState", &st2, 1)))

	st3 := execStartTimes[3]
	sh3a := makeStateHistory("sh-pg-dss-3a", "exec-pg-dss-003", "ProcessState", &st3, 1)
	sh3a.Status = "FAILED"
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh3a))
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-pg-dss-3b", "exec-pg-dss-003", "ValidateState", &st3, 2)))

	suite.Run("AllExecutions", func() {
		pairs, err := suite.repository.GetDistinctStateStatuses(suite.ctx, "sm-pg-dss-1", 10)
		require.NoError(suite.T(), err)
		assert.GreaterOrEqual(suite.T(), len(pairs), 4)

		found := make(map[string]bool)
		for _, p := range pairs {
			found[p.StateName+":"+p.Status] = true
		}
		assert.True(suite.T(), found["ProcessState:SUCCEEDED"])
		assert.True(suite.T(), found["ProcessState:FAILED"])
		assert.True(suite.T(), found["ValidateState:SUCCEEDED"])
		assert.True(suite.T(), found["ValidateState:FAILED"])
	})

	suite.Run("LimitedExecutions", func() {
		pairs, err := suite.repository.GetDistinctStateStatuses(suite.ctx, "sm-pg-dss-1", 1)
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), pairs, 2)
	})

	suite.Run("NonExistentStateMachine", func() {
		pairs, err := suite.repository.GetDistinctStateStatuses(suite.ctx, "sm-pg-nonexistent", 10)
		require.NoError(suite.T(), err)
		assert.Empty(suite.T(), pairs)
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — DeleteLinkedExecutions (raw Postgres)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *PostgresIntegrationTestSuite) TestDeleteLinkedExecutions() {
	baseTime := time.Now().Add(-1 * time.Hour)

	setupLinkedExecs := func() {
		suite.cleanupTestData()

		for i := 1; i <= 4; i++ {
			st := baseTime.Add(time.Duration(i) * time.Minute)
			require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
				ExecutionID:    fmt.Sprintf("exec-pg-dle-%03d", i),
				StateMachineID: fmt.Sprintf("sm-pg-dle-%d", (i-1)/2+1),
				Name:           fmt.Sprintf("pg-dle-test-%d", i),
				Status:         "SUCCEEDED",
				StartTime:      &st,
				CurrentState:   "Final",
			}))

			require.NoError(suite.T(), suite.repository.SaveLinkedExecution(suite.ctx, &LinkedExecutionRecord{
				ID:                     fmt.Sprintf("link-pg-dle-%03d", i),
				SourceStateMachineID:   fmt.Sprintf("sm-pg-dle-%d", (i-1)/2+1),
				SourceExecutionID:      fmt.Sprintf("exec-pg-dle-%03d", i),
				SourceStateName:        "SomeState",
				TargetStateMachineName: fmt.Sprintf("target-sm-%d", i%2+1),
				TargetExecutionID:      fmt.Sprintf("exec-pg-dle-target-%03d", i),
				CreatedAt:              baseTime.Add(time.Duration(i) * time.Minute),
			}))
		}
	}

	suite.Run("DeleteByIDs", func() {
		setupLinkedExecs()

		count, ids, err := suite.repository.DeleteLinkedExecutions(suite.ctx,
			[]string{"link-pg-dle-001", "link-pg-dle-003"}, nil)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(2), count)
		assert.ElementsMatch(suite.T(), []string{"link-pg-dle-001", "link-pg-dle-003"}, ids)

		remaining, err := suite.repository.ListLinkedExecutions(suite.ctx, nil)
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), remaining, 2)
	})

	suite.Run("DeleteByFilter", func() {
		setupLinkedExecs()

		count, ids, err := suite.repository.DeleteLinkedExecutions(suite.ctx, nil,
			&LinkedExecutionFilter{SourceStateMachineID: "sm-pg-dle-1"})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(2), count)
		assert.Nil(suite.T(), ids)

		remaining, err := suite.repository.ListLinkedExecutions(suite.ctx, nil)
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), remaining, 2)
	})

	suite.Run("SafetyCheck_EmptyIDsAndFilter", func() {
		_, _, err := suite.repository.DeleteLinkedExecutions(suite.ctx, nil, nil)
		require.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "refusing wide delete")
	})

	suite.Run("SafetyCheck_EmptyIDsAndEmptyFilter", func() {
		_, _, err := suite.repository.DeleteLinkedExecutions(suite.ctx, []string{}, &LinkedExecutionFilter{})
		require.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "refusing wide delete")
	})

	suite.Run("DeleteByIDs_NonExistent", func() {
		setupLinkedExecs()

		count, _, err := suite.repository.DeleteLinkedExecutions(suite.ctx,
			[]string{"nonexistent-link"}, nil)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(0), count)
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — GetExecutionStartTime (raw Postgres)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *PostgresIntegrationTestSuite) TestGetExecutionStartTime() {
	baseTime := time.Now().Add(-2 * time.Hour)

	exec := &ExecutionRecord{
		ExecutionID:    "exec-pg-est-001",
		StateMachineID: "sm-pg-est",
		Name:           "pg-est-test",
		Status:         "SUCCEEDED",
		StartTime:      &baseTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, exec))

	for i := 0; i < 3; i++ {
		sh := makeStateHistory(fmt.Sprintf("sh-pg-est-%d", i), "exec-pg-est-001", fmt.Sprintf("State%d", i), &baseTime, i)
		require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh))
	}

	suite.Run("WithRows", func() {
		t, err := suite.repository.GetExecutionStartTime(suite.ctx, "exec-pg-est-001")
		require.NoError(suite.T(), err)
		require.NotNil(suite.T(), t)
		assert.WithinDuration(suite.T(), baseTime, *t, time.Second)
	})

	suite.Run("WithoutRows", func() {
		_, err := suite.repository.GetExecutionStartTime(suite.ctx, "exec-pg-nonexistent")
		require.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "no state history found")
	})
}
