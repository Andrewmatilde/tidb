// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package oracles

import (
	"context"
	"github.com/tikv/pd/pkg/testutil"
	"go.uber.org/goleak"
	"testing"
	"time"

	. "github.com/pingcap/check"
	"github.com/pingcap/tidb/store/mockstore/mocktikv"
	"github.com/pingcap/tidb/store/tikv/oracle"
	pd "github.com/tikv/pd/client"
)

func TestT(t *testing.T) {
	TestingT(t)
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.LeakOptions...)
}

var _ = Suite(&clientTestSuite{})

type clientTestSuite struct {
	mvccStore *mocktikv.MVCCLevelDB
	cluster   *mocktikv.Cluster
	pdClient  pd.Client
	pd        *pdOracle
}

var oracleUpdateInterval time.Duration = 2000

func (s *clientTestSuite) SetUpSuite(c *C) {
	mvccStore, err := mocktikv.NewMVCCLevelDB("")
	c.Assert(err, IsNil)
	cluster := mocktikv.NewCluster(mvccStore)
	pdCli := mocktikv.NewPDClient(cluster)
	o, err := NewPdOracle(pdCli, oracleUpdateInterval*time.Millisecond)
	s.mvccStore = mvccStore
	s.cluster = cluster
	s.pd = o.(*pdOracle)
	s.pdClient = pdCli
	c.Assert(err, IsNil)
}

func (s *clientTestSuite) TearDownSuite(c *C) {
	err := s.mvccStore.Close()
	c.Assert(err, IsNil)
	s.pdClient.Close()
	s.pd.Close()
	time.Sleep(time.Second)
}

func TestPDOracle_UntilExpired(t *testing.T) {
	lockAfter, lockExp := 10, 15
	o := NewEmptyPDOracle()
	start := time.Now()
	SetEmptyPDOracleLastTs(o, oracle.ComposeTS(oracle.GetPhysical(start), 0))
	lockTs := oracle.ComposeTS(oracle.GetPhysical(start.Add(time.Duration(lockAfter)*time.Millisecond)), 1)
	waitTs := o.UntilExpired(lockTs, uint64(lockExp), &oracle.Option{TxnScope: oracle.GlobalTxnScope})
	if waitTs != int64(lockAfter+lockExp) {
		t.Errorf("waitTs shoulb be %d but got %d", int64(lockAfter+lockExp), waitTs)
	}
}

func (s *clientTestSuite) TestPdOracle_GetTimestamp(c *C) {
	opt := oracle.Option{}
	_, _ = s.pd.GetTimestamp(context.Background(), &opt)
}

func (s *clientTestSuite) TestPdOracle_GetStaleTimestamp(c *C) {
	now := time.Now()
	ts, err := s.pd.GetStaleTimestamp(context.Background(), 10)
	c.Assert(err, IsNil)

	duration := now.Sub(oracle.GetTimeFromTS(ts))
	c.Assert(duration, LessEqual, 12*time.Second)
	c.Assert(duration, GreaterEqual, 8*time.Second)

	_, err = s.pd.GetStaleTimestamp(context.Background(), 1e12)
	c.Assert(err, NotNil)

	_, err = s.pd.GetStaleTimestamp(context.Background(), -2)
	c.Assert(err, IsNil)

	for i := int64(3); i < 1e9; i += i/100 + 1 {
		now = time.Now()
		ts, err = s.pd.GetStaleTimestamp(context.Background(), i)
		c.Assert(err, IsNil)

		duration = now.Sub(oracle.GetTimeFromTS(ts))
		c.Assert(duration, LessEqual, time.Duration(i+2)*time.Second)
		c.Assert(duration, GreaterEqual, time.Duration(i-2)*time.Second)
	}
}
