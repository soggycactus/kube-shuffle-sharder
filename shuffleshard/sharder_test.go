package shuffleshard_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/soggycactus/kube-shuffle-sharder/shuffleshard"
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
	for i := 0; i < 20; i++ {
		endpoints = append(endpoints, fmt.Sprintf("group-%d", i))
	}

	sharder := shuffleshard.Sharder[string]{
		Endpoints:         endpoints,
		ReplicationFactor: 5,
		ShardStore:        store,
		ShardKeyFunc:      shuffleshard.HashShard,
	}

	shardCount := 0
	for {
		result, err := sharder.ShuffleShard(context.Background())
		if err != nil {
			t.Logf("shuffle shard failed: %v", err)
			t.Logf("created %d shards", shardCount)
			t.FailNow()
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
}
