package shuffleshard_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/soggycactus/kube-shuffle-sharder/shuffleshard"
)

const (
	// Run a test for 100 choose 3, which should result in 161,700 unique combinations
	NumEndpoints      = 100
	ReplicationFactor = 3
	ExpectedShards    = 161700
)

type MockShardStore struct {
	Store map[string]struct{}
}

func (m *MockShardStore) ShardExists(ctx context.Context, hash string) (bool, error) {
	if _, ok := m.Store[hash]; !ok {
		return false, nil
	}

	return true, nil
}

func TestSharder(t *testing.T) {
	store := &MockShardStore{
		Store: map[string]struct{}{},
	}

	var endpoints []string
	for i := 0; i < NumEndpoints; i++ {
		endpoints = append(endpoints, fmt.Sprintf("group-%d", i))
	}

	sharder := shuffleshard.Sharder[string]{
		Endpoints:         endpoints,
		ReplicationFactor: ReplicationFactor,
		ShardStore:        store,
		ShardKeyFunc:      shuffleshard.HashShard,
		Rand:              rand.New(rand.NewSource(time.Now().Unix())),
	}

	shardCount := 0
	for {
		result, err := sharder.ShuffleShard(context.Background())
		if err != nil {
			if err != shuffleshard.ErrNoShardsAvailable {
				t.Logf("shuffle shard failed: %v", err)
				t.Logf("created %d shards", shardCount)
				t.FailNow()
			}
			break
		}

		hash, err := shuffleshard.HashShard(result)
		if err != nil {
			t.Logf("failed to hash shard: %v", err)
			t.Logf("created %d shards", shardCount)
			t.FailNow()
		}

		store.Store[hash] = struct{}{}

		shardCount += 1
	}

	if shardCount != ExpectedShards {
		t.Logf("incorrect shard count: got %d, expected %d", shardCount, ExpectedShards)
		t.FailNow()
	}
}
