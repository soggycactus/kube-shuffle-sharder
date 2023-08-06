package shuffleshard

import (
	"context"
	"errors"
	"math/rand"
	"sort"
)

var ErrNoShardsAvailable = errors.New("no shards available")
var ErrShardAlreadyExists = errors.New("shard already exists")
var ErrTooManyOverlaps = errors.New("shard has too many overlapping endpoints")

type ShardStore[T any] interface {
	ShardExists(ctx context.Context, shardHash string) (bool, error)
	ShardExistsWithEndpoints(ctx context.Context, endpoints []T) bool
	NumEdges(T) int
}

type Sharder[T any] struct {
	// Endpoints are a list of endpoints to generate shuffle shards from
	Endpoints []T

	// ReplicationFactor is the size of an individual shuffle shard
	ReplicationFactor int

	// ShardStore is a storage of all existing shuffle shards.
	// Sharder queries this store to ensure that a shuffle shard isn't allocated twice.
	// ShuffleShards are stored by a unique, deterministic key; a hash of the shard.
	ShardStore ShardStore[T]

	// ShardKeyFunc is a function that receives a shuffle shard & returns a hash of its value.
	// It is used to lookup a shard for existence in the ShardStore.
	// It must not modify the received shuffle shard, instead make a deep copy of the parameter.
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

func (s *Sharder[T]) ShuffleShardWithoutOverlap(ctx context.Context, maxOverlap int) ([]T, error) {
	// Shuffle the order of endpoints for better performance
	sort.Slice(s.Endpoints, func(i, j int) bool {
		return s.ShardStore.NumEdges(s.Endpoints[i]) < s.ShardStore.NumEdges(s.Endpoints[j])
	})
	return s.backtrackWithoutOverlap(ctx, []T{}, s.Endpoints, maxOverlap, make(map[string]bool))
}

func (s *Sharder[T]) backtrackWithoutOverlap(ctx context.Context, cursor, endpoints []T, maxOverlap int, memo map[string]bool) ([]T, error) {
	if len(cursor) >= maxOverlap {
		combinations := s.combinations(cursor, maxOverlap)

		for _, combo := range combinations {
			comboHash, err := s.ShardKeyFunc(combo)
			if err != nil {
				return nil, err
			}

			if overlap, ok := memo[comboHash]; ok {
				if overlap {
					return nil, ErrTooManyOverlaps
				}
				continue
			}

			overlap := s.ShardStore.ShardExistsWithEndpoints(context.Background(), combo)
			memo[comboHash] = overlap

			if overlap {
				return nil, ErrTooManyOverlaps
			}
		}
	}

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
		result, err := s.backtrackWithoutOverlap(ctx, cursor, newEndpoints, maxOverlap, memo)
		if err != nil {
			cursor = cursor[:len(cursor)-1]
			continue
		}

		return result, nil
	}

	return nil, ErrNoShardsAvailable
}

func (s *Sharder[T]) combinations(cursor []T, size int) [][]T {
	n := len(cursor)
	data := make([]T, size)
	var result [][]T
	s.combinationsUtil(cursor, data, 0, n-1, 0, size, &result)
	return result
}

func (s *Sharder[T]) combinationsUtil(arr []T, data []T, start int, end int, index int, r int, result *[][]T) {
	if index == r {
		// Append a copy of the combination to the result slice
		comb := make([]T, r)
		copy(comb, data)
		*result = append(*result, comb)
		return
	}

	for i := start; i <= end && end-i+1 >= r-index; i++ {
		data[index] = arr[i]
		s.combinationsUtil(arr, data, i+1, end, index+1, r, result)
	}
}
