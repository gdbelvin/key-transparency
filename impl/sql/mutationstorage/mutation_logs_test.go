// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mutationstorage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/keytransparency/core/adminserver"
	"github.com/google/keytransparency/core/integration/storagetest"
	"github.com/google/keytransparency/core/keyserver"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/google/keytransparency/core/api/v1/keytransparency_go_proto"
	_ "github.com/mattn/go-sqlite3"
)

func newForTest(ctx context.Context, t testing.TB, dirID string, logIDs ...int64) *Mutations {
	m, err := New(newDB(t))
	if err != nil {
		t.Fatalf("Failed to create mutation storage: %v", err)
	}
	if err := m.AddLogs(ctx, dirID, logIDs...); err != nil {
		t.Fatalf("AddLogs(): %v", err)
	}
	return m
}

func TestMutationLogsIntegration(t *testing.T) {
	storagetest.RunMutationLogsTests(t,
		func(ctx context.Context, t *testing.T, dirID string, logIDs ...int64) keyserver.MutationLogs {
			return newForTest(ctx, t, dirID, logIDs...)
		})
}

func TestLogsAdminIntegration(t *testing.T) {
	storagetest.RunLogsAdminTests(t,
		func(ctx context.Context, t *testing.T, dirID string, logIDs ...int64) adminserver.LogsAdmin {
			return newForTest(ctx, t, dirID, logIDs...)
		})
}

func TestRandLog(t *testing.T) {
	ctx := context.Background()
	directoryID := "TestRandLog"

	for _, tc := range []struct {
		desc     string
		send     []int64
		wantCode codes.Code
		wantLogs map[int64]bool
	}{
		{desc: "no rows", wantCode: codes.NotFound, wantLogs: map[int64]bool{}},
		{desc: "one row", send: []int64{10}, wantLogs: map[int64]bool{10: true}},
		{desc: "second", send: []int64{1, 2, 3}, wantLogs: map[int64]bool{
			1: true,
			2: true,
			3: true,
		}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			m := newForTest(ctx, t, directoryID, tc.send...)
			logs := make(map[int64]bool)
			for i := 0; i < 10*len(tc.wantLogs); i++ {
				logID, err := m.randLog(ctx, directoryID)
				if got, want := status.Code(err), tc.wantCode; got != want {
					t.Errorf("randLog(): %v, want %v", got, want)
				}
				if err != nil {
					break
				}
				logs[logID] = true
			}
			if got, want := logs, tc.wantLogs; !cmp.Equal(got, want) {
				t.Errorf("logs: %v, want %v", got, want)
			}
		})
	}
}

func BenchmarkSend(b *testing.B) {
	ctx := context.Background()
	directoryID := "BenchmarkSend"
	logID := int64(1)
	m := newForTest(ctx, b, directoryID, logID)

	update := &pb.EntryUpdate{Mutation: &pb.SignedEntry{Entry: []byte("xxxxxxxxxxxxxxxxxx")}}
	for _, tc := range []struct {
		batch int
	}{
		{batch: 1},
		{batch: 2},
		{batch: 4},
		{batch: 8},
		{batch: 16},
		{batch: 32},
		{batch: 64},
		{batch: 128},
		{batch: 256},
	} {
		b.Run(fmt.Sprintf("%d", tc.batch), func(b *testing.B) {
			updates := make([]*pb.EntryUpdate, 0, tc.batch)
			for i := 0; i < tc.batch; i++ {
				updates = append(updates, update)
			}
			for n := 0; n < b.N; n++ {
				if _, err := m.Send(ctx, directoryID, updates...); err != nil {
					b.Errorf("Send(): %v", err)
				}
			}
		})
	}
}

func TestSend(t *testing.T) {
	ctx := context.Background()

	directoryID := "TestSend"
	m := newForTest(ctx, t, directoryID, 1, 2)
	update := []byte("bar")
	ts1 := time.Now()
	ts2 := ts1.Add(time.Microsecond)
	ts3 := ts2.Add(time.Microsecond)

	// Test cases are cumulative. Earlier test caes setup later test cases.
	for _, tc := range []struct {
		desc     string
		ts       time.Time
		wantCode codes.Code
	}{
		// Enforce timestamp uniqueness.
		{desc: "First", ts: ts2},
		{desc: "Second", ts: ts2, wantCode: codes.Aborted},
		// Enforce a monotonically increasing timestamp
		{desc: "Old", ts: ts1, wantCode: codes.Aborted},
		{desc: "New", ts: ts3},
	} {
		err := m.send(ctx, tc.ts, directoryID, 1, update, update)
		if got, want := status.Code(err), tc.wantCode; got != want {
			t.Errorf("%v: send(): %v, got: %v, want %v", tc.desc, err, got, want)
		}
	}
}

func TestWatermark(t *testing.T) {
	ctx := context.Background()
	directoryID := "TestWatermark"
	logIDs := []int64{1, 2}
	m := newForTest(ctx, t, directoryID, logIDs...)
	update := []byte("bar")

	startTS := time.Date(1990, 2, 3, 4 /*hour*/, 5, 6, 7, time.UTC)
	// Add an item to both logs every 1 millisecond for 10 milliseconds.
	for ts := startTS; ts.Before(startTS.Add(10 * time.Microsecond)); ts = ts.Add(time.Microsecond) {
		for _, logID := range logIDs {
			if err := m.send(ctx, ts, directoryID, logID, update); err != nil {
				t.Fatalf("m.send(%v): %v", logID, err)
			}
		}
	}

	start := startTS
	for _, tc := range []struct {
		desc      string
		logID     int64
		start     time.Time
		batchSize int32
		count     int32
		want      time.Time
	}{
		{desc: "log1 max", logID: 1, batchSize: 100, want: start.Add(10 * time.Microsecond), count: 10},
		{desc: "log2 max", logID: 2, batchSize: 100, want: start.Add(10 * time.Microsecond), count: 10},
		{desc: "batch0", logID: 1, batchSize: 0},
		{desc: "keephighwatermark", logID: 1, start: startTS.Add(55 * time.Microsecond), batchSize: 0, want: startTS.Add(55 * time.Microsecond)},
		{desc: "batch5", logID: 1, batchSize: 5, want: start.Add(5 * time.Microsecond), count: 5},
		{desc: "start1", logID: 1, start: start.Add(2 * time.Microsecond), batchSize: 5, want: start.Add(7 * time.Microsecond), count: 5},
		{desc: "start8", logID: 1, start: start.Add(8 * time.Microsecond), batchSize: 5, want: start.Add(10 * time.Microsecond), count: 2},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			count, got, err := m.HighWatermark(ctx, directoryID, tc.logID, watermark(tc.start), tc.batchSize)
			if err != nil {
				t.Errorf("highWatermark(): %v", err)
			}
			if want := watermark(tc.want); got != want {
				t.Errorf("highWatermark(%v) high: %v, want %v", tc.start, got, want)
			}
			if count != tc.count {
				t.Errorf("highWatermark(%v) count: %v, want %v", tc.start, count, tc.count)
			}
		})
	}
}