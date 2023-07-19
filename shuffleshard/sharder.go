package shuffleshard

import (
	"context"
	"errors"
	"math/rand"
)

var ErrNoShardsAvailable = errors.New("no shards available")
var ErrShardAlreadyExists = errors.New("shard already exists")

type ShardStore interface {
	ShardExists(ctx context.Context, shardHash string) (bool, error)
}

type Sharder[T any] struct {
	// Endpoints are a list of endpoints to generate shuffle shards from
	Endpoints []T
	// ReplicationFactor is the size of an individual shuffle shard
	ReplicationFactor int

	// ShardStore is a storage of all existing shuffle shards.
	// Sharder queries this store to ensure that a shuffle shard isn't allocated twice.
	ShardStore ShardStore

	// ShardKeyFunc is a function that receives a shuffle shard & returns a hash of its value.
	// It is used to lookup a shard for existence in the ShardStore.
	ShardKeyFunc func([]T) (string, error)

	Rand *rand.Rand
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
