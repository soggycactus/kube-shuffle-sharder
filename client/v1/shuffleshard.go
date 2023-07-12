package v1

import (
	"context"

	v1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ShuffleShardClient struct {
	Client client.Client
}

func (s *ShuffleShardClient) CreateShuffleShard(ctx context.Context, shard v1.ShuffleShard, opts client.CreateOption) (*v1.ShuffleShard, error) {
	err := s.Client.Create(ctx, &shard, opts)
	return &shard, err
}

func (s *ShuffleShardClient) ListShuffleShards(ctx context.Context, opts client.ListOption) (*v1.ShuffleShardList, error) {
	var shuffleShardList v1.ShuffleShardList
	if err := s.Client.List(ctx, &shuffleShardList, opts); err != nil {
		return nil, err
	}

	return &shuffleShardList, nil
}
