package shuffleshard_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/soggycactus/kube-shuffle-sharder/shuffleshard"
)

const (
	// Run a test for 20 choose 5, which should result in 15,504 unique combinations
	NumEndpoints      = 20
	ReplicationFactor = 5
	ExpectedShards    = 15504
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

func HashShard(shard []string) (string, error) {
	shardCopy := make([]string, len(shard))
	copy(shardCopy, shard)
	sort.Strings(shardCopy)

	nodeGroups := strings.Join(shardCopy, "")
	hasher := sha256.New()
	_, err := hasher.Write([]byte(nodeGroups))
	if err != nil {
		return "", err
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash, nil
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
		ShardKeyFunc:      HashShard,
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

		hash, err := HashShard(result)
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
