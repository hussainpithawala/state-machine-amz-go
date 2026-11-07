package repository

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — ExecutionFilter extensions
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *GormPostgresIntegrationTestSuite) TestExecutionFilter_ExecutionID() {
	baseTime := time.Now().Add(-1 * time.Hour)

	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-eid-%03d", i),
			StateMachineID: "sm-eid",
			Name:           fmt.Sprintf("eid-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		ExecutionID: "exec-eid-002",
	})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), results, 1)
	assert.Equal(suite.T(), "exec-eid-002", results[0].ExecutionID)

	count, err := suite.repository.CountExecutions(suite.ctx, &ExecutionFilter{
		ExecutionID: "exec-eid-002",
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count)
}

func (suite *GormPostgresIntegrationTestSuite) TestExecutionFilter_StateName_StateStatus() {
	baseTime := time.Now().Add(-2 * time.Hour)

	execStartTimes := make(map[int]time.Time)
	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		execStartTimes[i] = st
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-sn-%03d", i),
			StateMachineID: "sm-sn",
			Name:           fmt.Sprintf("sn-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	// exec-sn-001: has ProcessState SUCCEEDED
	st1 := execStartTimes[1]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-sn-1", "exec-sn-001", "ProcessState", &st1, 1)))

	// exec-sn-002: has ProcessState FAILED
	st2 := execStartTimes[2]
	sh2 := makeStateHistory("sh-sn-2", "exec-sn-002", "ProcessState", &st2, 1)
	sh2.Status = "FAILED"
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh2))

	// exec-sn-003: has DifferentState SUCCEEDED
	st3 := execStartTimes[3]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-sn-3", "exec-sn-003", "DifferentState", &st3, 1)))

	suite.Run("FilterByStateName", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-sn",
			StateName:      "ProcessState",
		})
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), results, 2)
		ids := collectExecutionIDs(results)
		assert.Contains(suite.T(), ids, "exec-sn-001")
		assert.Contains(suite.T(), ids, "exec-sn-002")
	})

	suite.Run("FilterByStateNameAndStatus", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-sn",
			StateName:      "ProcessState",
			StateStatus:    "SUCCEEDED",
		})
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), results, 1)
		assert.Equal(suite.T(), "exec-sn-001", results[0].ExecutionID)
	})

	suite.Run("FilterByStateStatusOnly", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-sn",
			StateStatus:    "FAILED",
		})
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), results, 1)
		assert.Equal(suite.T(), "exec-sn-002", results[0].ExecutionID)
	})

	suite.Run("CountMatchesListFilter", func() {
		count, err := suite.repository.CountExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-sn",
			StateName:      "ProcessState",
		})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(2), count)
	})
}

func (suite *GormPostgresIntegrationTestSuite) TestExecutionFilter_SortByOrder() {
	baseTime := time.Now().Add(-3 * time.Hour)

	statuses := []string{"FAILED", "RUNNING", "SUCCEEDED"}
	for i, status := range statuses {
		st := baseTime.Add(time.Duration(i*10) * time.Minute)
		endT := st.Add(time.Duration((3-i)*5) * time.Minute)
		rec := &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-sort-%03d", i+1),
			StateMachineID: "sm-sort",
			Name:           fmt.Sprintf("sort-test-%d", i+1),
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
			StateMachineID: "sm-sort",
			SortBy:         "startTime",
			Order:          "desc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), results, 3)
		assert.Equal(suite.T(), "exec-sort-003", results[0].ExecutionID)
		assert.Equal(suite.T(), "exec-sort-001", results[2].ExecutionID)
	})

	suite.Run("SortByStartTimeAsc", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-sort",
			SortBy:         "startTime",
			Order:          "asc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), results, 3)
		assert.Equal(suite.T(), "exec-sort-001", results[0].ExecutionID)
		assert.Equal(suite.T(), "exec-sort-003", results[2].ExecutionID)
	})

	suite.Run("SortByStatus", func() {
		results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-sort",
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
			StateMachineID: "sm-sort",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), results, 3)
		// Default is start_time DESC
		assert.Equal(suite.T(), "exec-sort-003", results[0].ExecutionID)
	})
}

func (suite *GormPostgresIntegrationTestSuite) TestExecutionFilter_StartBefore() {
	baseTime := time.Now().Add(-2 * time.Hour)

	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i*20) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-sb-%03d", i),
			StateMachineID: "sm-sb",
			Name:           fmt.Sprintf("sb-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-sb",
		StartBefore:    baseTime.Add(45 * time.Minute),
	})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), results, 2)
	ids := collectExecutionIDs(results)
	assert.Contains(suite.T(), ids, "exec-sb-001")
	assert.Contains(suite.T(), ids, "exec-sb-002")
	assert.NotContains(suite.T(), ids, "exec-sb-003")
}

func (suite *GormPostgresIntegrationTestSuite) TestExecutionFilter_StartAfterAndStartBefore_Combined() {
	baseTime := time.Now().Add(-3 * time.Hour)

	for i := 1; i <= 5; i++ {
		st := baseTime.Add(time.Duration(i*10) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-range-%03d", i),
			StateMachineID: "sm-range",
			Name:           fmt.Sprintf("range-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	results, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-range",
		StartAfter:     baseTime.Add(15 * time.Minute),
		StartBefore:    baseTime.Add(35 * time.Minute),
	})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), results, 2)
	ids := collectExecutionIDs(results)
	assert.Contains(suite.T(), ids, "exec-range-002")
	assert.Contains(suite.T(), ids, "exec-range-003")
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — LinkedExecutionFilter sort/order
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *GormPostgresIntegrationTestSuite) TestLinkedExecutionFilter_SortByOrder() {
	baseTime := time.Now().Add(-1 * time.Hour)

	smIDs := []string{"sm-b", "sm-a", "sm-c"}
	targetNames := []string{"TargetZ", "TargetA", "TargetM"}

	for i := 0; i < 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-le-sort-%03d", i+1),
			StateMachineID: smIDs[i],
			Name:           fmt.Sprintf("le-sort-%d", i+1),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))

		require.NoError(suite.T(), suite.repository.SaveLinkedExecution(suite.ctx, &LinkedExecutionRecord{
			ID:                     fmt.Sprintf("link-le-sort-%03d", i+1),
			SourceStateMachineID:   smIDs[i],
			SourceExecutionID:      fmt.Sprintf("exec-le-sort-%03d", i+1),
			TargetStateMachineName: targetNames[i],
			TargetExecutionID:      fmt.Sprintf("exec-le-sort-target-%03d", i+1),
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
		assert.Equal(suite.T(), "link-le-sort-001", links[0].ID)
		assert.Equal(suite.T(), "link-le-sort-003", links[2].ID)
	})

	suite.Run("SortBySourceStateMachineIdAsc", func() {
		links, err := suite.repository.ListLinkedExecutions(suite.ctx, &LinkedExecutionFilter{
			SortBy: "sourceStateMachineId",
			Order:  "asc",
		})
		require.NoError(suite.T(), err)
		require.Len(suite.T(), links, 3)
		assert.Equal(suite.T(), "sm-a", links[0].SourceStateMachineID)
		assert.Equal(suite.T(), "sm-c", links[2].SourceStateMachineID)
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
// Stage 1 — ListStateHistory (cursor pagination)
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *GormPostgresIntegrationTestSuite) TestListStateHistory() {
	baseTime := time.Now().Add(-2 * time.Hour)

	exec := &ExecutionRecord{
		ExecutionID:    "exec-lsh-001",
		StateMachineID: "sm-lsh",
		Name:           "lsh-test",
		Status:         "SUCCEEDED",
		StartTime:      &baseTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, exec))

	for i := 0; i < 5; i++ {
		sh := makeStateHistory(fmt.Sprintf("sh-lsh-%d", i), "exec-lsh-001", fmt.Sprintf("State%d", i), &baseTime, i)
		require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh))
	}

	suite.Run("AllRecords_NoLimit", func() {
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-lsh-001", nil, 0, "")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 5)
		assert.Equal(suite.T(), 0, records[0].SequenceNumber)
		assert.Equal(suite.T(), 4, records[4].SequenceNumber)
	})

	suite.Run("FirstPage", func() {
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-lsh-001", nil, 2, "asc")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 2)
		assert.Equal(suite.T(), 0, records[0].SequenceNumber)
		assert.Equal(suite.T(), 1, records[1].SequenceNumber)
	})

	suite.Run("CursorPage", func() {
		afterSeq := 1
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-lsh-001", &afterSeq, 2, "asc")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 2)
		assert.Equal(suite.T(), 2, records[0].SequenceNumber)
		assert.Equal(suite.T(), 3, records[1].SequenceNumber)
	})

	suite.Run("CursorPage_LastPage", func() {
		afterSeq := 3
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-lsh-001", &afterSeq, 10, "asc")
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(5), total)
		assert.Len(suite.T(), records, 1)
		assert.Equal(suite.T(), 4, records[0].SequenceNumber)
	})

	suite.Run("DescOrder", func() {
		records, _, err := suite.repository.ListStateHistory(suite.ctx, "exec-lsh-001", nil, 3, "desc")
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), records, 3)
		assert.Equal(suite.T(), 4, records[0].SequenceNumber)
		assert.Equal(suite.T(), 3, records[1].SequenceNumber)
		assert.Equal(suite.T(), 2, records[2].SequenceNumber)
	})

	suite.Run("EmptyExecution", func() {
		records, total, err := suite.repository.ListStateHistory(suite.ctx, "exec-nonexistent", nil, 0, "")
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
// Stage 1 — GetDashboardStats
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *GormPostgresIntegrationTestSuite) TestGetDashboardStats() {
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

		// Create state machines
		for i := 1; i <= 2; i++ {
			require.NoError(suite.T(), suite.repository.SaveStateMachine(suite.ctx, &StateMachineRecord{
				ID:         fmt.Sprintf("sm-dash-%d", i),
				Name:       fmt.Sprintf("dash-sm-%d", i),
				Definition: "{}",
				Version:    "1.0",
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}))
		}

		now := time.Now()

		// Succeeded executions (within 7 days for duration stats)
		for i := 1; i <= 3; i++ {
			st := now.Add(time.Duration(-i) * time.Hour)
			et := st.Add(30 * time.Second)
			require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
				ExecutionID:    fmt.Sprintf("exec-dash-succ-%d", i),
				StateMachineID: "sm-dash-1",
				Name:           fmt.Sprintf("dash-succ-%d", i),
				Status:         "SUCCEEDED",
				StartTime:      &st,
				EndTime:        &et,
				CurrentState:   "Final",
			}))
		}

		// Failed executions
		for i := 1; i <= 2; i++ {
			st := now.Add(time.Duration(-i*2) * time.Hour)
			require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
				ExecutionID:    fmt.Sprintf("exec-dash-fail-%d", i),
				StateMachineID: "sm-dash-1",
				Name:           fmt.Sprintf("dash-fail-%d", i),
				Status:         "FAILED",
				StartTime:      &st,
				CurrentState:   "ErrorState",
				Error:          fmt.Sprintf("error-%d", i),
			}))
		}

		// Running execution
		st := now.Add(-30 * time.Minute)
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    "exec-dash-run-1",
			StateMachineID: "sm-dash-2",
			Name:           "dash-run-1",
			Status:         "RUNNING",
			StartTime:      &st,
			CurrentState:   "Processing",
		}))

		stats, err := suite.repository.GetDashboardStats(suite.ctx)
		require.NoError(suite.T(), err)
		require.NotNil(suite.T(), stats)

		assert.Equal(suite.T(), int64(2), stats.TotalStateMachines)
		assert.Equal(suite.T(), int64(6), stats.TotalExecutions)

		// Status counts
		statusMap := make(map[string]int64)
		for _, sc := range stats.StatusCounts {
			statusMap[sc.Status] = sc.Count
		}
		assert.Equal(suite.T(), int64(3), statusMap["SUCCEEDED"])
		assert.Equal(suite.T(), int64(2), statusMap["FAILED"])
		assert.Equal(suite.T(), int64(1), statusMap["RUNNING"])

		// Duration stats (from 3 SUCCEEDED executions)
		require.NotNil(suite.T(), stats.DurationStats)
		require.NotNil(suite.T(), stats.DurationStats.AvgDuration)
		assert.InDelta(suite.T(), 30.0, *stats.DurationStats.AvgDuration, 1.0)

		// Recent failures
		assert.Len(suite.T(), stats.RecentFailures, 2)
		assert.Equal(suite.T(), "FAILED", stats.RecentFailures[0].Status)

		// Timestamp is from DB
		assert.False(suite.T(), stats.Timestamp.IsZero())
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — GetDistinctStateStatuses
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *GormPostgresIntegrationTestSuite) TestGetDistinctStateStatuses() {
	baseTime := time.Now().Add(-2 * time.Hour)

	execStartTimes := make(map[int]time.Time)
	for i := 1; i <= 3; i++ {
		st := baseTime.Add(time.Duration(i) * time.Minute)
		execStartTimes[i] = st
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-dss-%03d", i),
			StateMachineID: "sm-dss-1",
			Name:           fmt.Sprintf("dss-test-%d", i),
			Status:         "SUCCEEDED",
			StartTime:      &st,
			CurrentState:   "Final",
		}))
	}

	// exec-dss-001: ProcessState SUCCEEDED, ValidateState FAILED
	st1 := execStartTimes[1]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-dss-1a", "exec-dss-001", "ProcessState", &st1, 1)))
	sh1b := makeStateHistory("sh-dss-1b", "exec-dss-001", "ValidateState", &st1, 2)
	sh1b.Status = "FAILED"
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh1b))

	// exec-dss-002: ProcessState SUCCEEDED
	st2 := execStartTimes[2]
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-dss-2", "exec-dss-002", "ProcessState", &st2, 1)))

	// exec-dss-003: ProcessState FAILED, ValidateState SUCCEEDED
	st3 := execStartTimes[3]
	sh3a := makeStateHistory("sh-dss-3a", "exec-dss-003", "ProcessState", &st3, 1)
	sh3a.Status = "FAILED"
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh3a))
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx,
		makeStateHistory("sh-dss-3b", "exec-dss-003", "ValidateState", &st3, 2)))

	suite.Run("AllExecutions", func() {
		pairs, err := suite.repository.GetDistinctStateStatuses(suite.ctx, "sm-dss-1", 10)
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
		// Only consider the most recent execution (exec-dss-003)
		pairs, err := suite.repository.GetDistinctStateStatuses(suite.ctx, "sm-dss-1", 1)
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), pairs, 2)
	})

	suite.Run("NonExistentStateMachine", func() {
		pairs, err := suite.repository.GetDistinctStateStatuses(suite.ctx, "sm-nonexistent", 10)
		require.NoError(suite.T(), err)
		assert.Empty(suite.T(), pairs)
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// Stage 1 — DeleteLinkedExecutions
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *GormPostgresIntegrationTestSuite) TestDeleteLinkedExecutions() {
	baseTime := time.Now().Add(-1 * time.Hour)

	setupLinkedExecs := func() {
		suite.cleanupTestData()

		for i := 1; i <= 4; i++ {
			st := baseTime.Add(time.Duration(i) * time.Minute)
			require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
				ExecutionID:    fmt.Sprintf("exec-dle-%03d", i),
				StateMachineID: fmt.Sprintf("sm-dle-%d", (i-1)/2+1),
				Name:           fmt.Sprintf("dle-test-%d", i),
				Status:         "SUCCEEDED",
				StartTime:      &st,
				CurrentState:   "Final",
			}))

			require.NoError(suite.T(), suite.repository.SaveLinkedExecution(suite.ctx, &LinkedExecutionRecord{
				ID:                     fmt.Sprintf("link-dle-%03d", i),
				SourceStateMachineID:   fmt.Sprintf("sm-dle-%d", (i-1)/2+1),
				SourceExecutionID:      fmt.Sprintf("exec-dle-%03d", i),
				SourceStateName:        "SomeState",
				TargetStateMachineName: fmt.Sprintf("target-sm-%d", i%2+1),
				TargetExecutionID:      fmt.Sprintf("exec-dle-target-%03d", i),
				CreatedAt:              baseTime.Add(time.Duration(i) * time.Minute),
			}))
		}
	}

	suite.Run("DeleteByIDs", func() {
		setupLinkedExecs()

		count, ids, err := suite.repository.DeleteLinkedExecutions(suite.ctx,
			[]string{"link-dle-001", "link-dle-003"}, nil)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), int64(2), count)
		assert.ElementsMatch(suite.T(), []string{"link-dle-001", "link-dle-003"}, ids)

		remaining, err := suite.repository.ListLinkedExecutions(suite.ctx, nil)
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), remaining, 2)
	})

	suite.Run("DeleteByFilter", func() {
		setupLinkedExecs()

		count, ids, err := suite.repository.DeleteLinkedExecutions(suite.ctx, nil,
			&LinkedExecutionFilter{SourceStateMachineID: "sm-dle-1"})
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
// Stage 1 — GetExecutionStartTime
// ═══════════════════════════════════════════════════════════════════════════════

func (suite *GormPostgresIntegrationTestSuite) TestGetExecutionStartTime() {
	baseTime := time.Now().Add(-2 * time.Hour)

	exec := &ExecutionRecord{
		ExecutionID:    "exec-est-001",
		StateMachineID: "sm-est",
		Name:           "est-test",
		Status:         "SUCCEEDED",
		StartTime:      &baseTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, exec))

	// Create state history with different sequence numbers
	for i := 0; i < 3; i++ {
		sh := makeStateHistory(fmt.Sprintf("sh-est-%d", i), "exec-est-001", fmt.Sprintf("State%d", i), &baseTime, i)
		require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, sh))
	}

	suite.Run("WithRows", func() {
		t, err := suite.repository.GetExecutionStartTime(suite.ctx, "exec-est-001")
		require.NoError(suite.T(), err)
		require.NotNil(suite.T(), t)
		assert.WithinDuration(suite.T(), baseTime, *t, time.Second)
	})

	suite.Run("WithoutRows", func() {
		_, err := suite.repository.GetExecutionStartTime(suite.ctx, "exec-nonexistent")
		require.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "no state history found")
	})
}
