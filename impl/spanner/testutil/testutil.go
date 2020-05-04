// Copyright 2020 Google Inc. All Rights Reserved.
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

// Package testutil helps with tests against spanner.
package testutil

import (
	"context"
	"flag"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/spannertest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	databasepb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
)

var testDBFlag = flag.String("test_db", "", "Fully-qualified database name to test against; empty means use an in-memory fake.")

func dbName() string {
	if *testDBFlag != "" {
		return *testDBFlag
	}
	return "projects/fake-proj/instances/fake-instance/databases/fake-db"
}

func Client(ctx context.Context, t *testing.T) (*spanner.Client, *database.DatabaseAdminClient, func()) {
	t.Helper()
	if *testDBFlag != "" {
		t.Logf("Using real Spanner DB %s", *testDBFlag)
		dialOpt := option.WithGRPCDialOption(grpc.WithTimeout(5 * time.Second))
		client, err := spanner.NewClient(ctx, *testDBFlag, dialOpt)
		if err != nil {
			t.Fatalf("Connecting to %s: %v", *testDBFlag, err)
		}
		adminClient, err := database.NewDatabaseAdminClient(ctx, dialOpt)
		if err != nil {
			client.Close()
			t.Fatalf("Connecting DB admin client: %v", err)
		}
		return client, adminClient, func() { client.Close(); adminClient.Close() }
	}

	// Don't use SPANNER_EMULATOR_HOST because we need the raw connection for
	// the database admin client anyway.

	t.Logf("Using in-memory fake Spanner DB")
	srv, err := spannertest.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("Starting in-memory fake: %v", err)
	}
	srv.SetLogger(t.Logf)
	dialCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, srv.Addr, grpc.WithInsecure())
	if err != nil {
		srv.Close()
		t.Fatalf("Dialing in-memory fake: %v", err)
	}
	client, err := spanner.NewClient(ctx, dbName(), option.WithGRPCConn(conn))
	if err != nil {
		srv.Close()
		t.Fatalf("Connecting to in-memory fake: %v", err)
	}
	adminClient, err := database.NewDatabaseAdminClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		srv.Close()
		t.Fatalf("Connecting to in-memory fake DB admin: %v", err)
	}
	return client, adminClient, func() {
		client.Close()
		adminClient.Close()
		conn.Close()
		srv.Close()
	}
}

func UpdateDDL(ctx context.Context, t *testing.T, adminClient *database.DatabaseAdminClient, statements ...string) error {
	t.Helper()
	t.Logf("DDL update: %q", statements)
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbName(),
		Statements: statements,
	})
	if err != nil {
		t.Fatalf("Starting DDL update: %v", err)
	}
	return op.Wait(ctx)
}
