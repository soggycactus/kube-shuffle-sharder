package shuffleshard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/rand"
	"sort"
	"strings"
)

var ErrNoShardsAvailable = errors.New("no shards available")
var ErrShardAlreadyExists = errors.New("shard already exists")

type ShardStore interface {
	ShardExists(ctx context.Context, shardHash string) (bool, error)
}

type Sharder[T any] struct {
	Endpoints         []T
	ReplicationFactor int
	ShardStore        ShardStore
	ShardKeyFunc      func([]T) (string, error)
	Rand              *rand.Rand
}

func (s *Sharder[T]) ShuffleShard(ctx context.Context) ([]T, error) {
	// Shuffle the order of endpoints for better performance
	s.Rand.Shuffle(len(s.Endpoints), func(i, j int) {
		s.Endpoints[i], s.Endpoints[j] = s.Endpoints[j], s.Endpoints[i]
	})
	return s.backtrack(ctx, []T{}, s.Endpoints)
}

func (s *Sharder[T]) backtrack(ctx context.Context, cursor, endpoints []T) ([]T, error) {
	if len(cursor) == s.ReplicationFactor {
		hash, err := s.ShardKeyFunc(cursor)
		if err != nil {
			return nil, err
		}

		ok, err := s.ShardStore.ShardExists(ctx, hash)
		if err != nil {
			return nil, err
		}

		if ok {
			return nil, ErrShardAlreadyExists
		}

		return cursor, nil
	}

	for index, endpoint := range endpoints {
		// only pass the remaining available endpoints further down the stack
		newEndpoints := make([]T, len(endpoints)-(index+1))
		copy(newEndpoints, endpoints[index+1:])

		// add the current endpoint to the cursor
		cursor = append(cursor, endpoint)
		result, err := s.backtrack(ctx, cursor, newEndpoints)
		if err != nil {
			cursor = cursor[:len(cursor)-1]
			continue
		}

		return result, nil
	}

	return nil, ErrNoShardsAvailable
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
