package v1

import (
	"context"

	v1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewShuffleShardClient() (*ShuffleShardClient, error) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	kubeconfig, err := controllerruntime.GetConfig()
	if err != nil {
		return nil, err
	}

	kubeclient, err := client.New(kubeconfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &ShuffleShardClient{Client: kubeclient}, nil
}

type ShuffleShardClient struct {
	client.Client
}

func (s *ShuffleShardClient) CreateShuffleShard(ctx context.Context, shard v1.ShuffleShard, opts client.CreateOption) (*v1.ShuffleShard, error) {
	err := s.Create(ctx, &shard, opts)
	return &shard, err
}

func (s *ShuffleShardClient) ListShuffleShards(ctx context.Context, opts client.ListOption) (*v1.ShuffleShardList, error) {
	var shuffleShardList v1.ShuffleShardList
	if err := s.List(ctx, &shuffleShardList, opts); err != nil {
		return nil, err
	}

	return &shuffleShardList, nil
}
