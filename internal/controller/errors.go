package controller

import "errors"

var ErrUnableToCastNode = errors.New("unable to cast interface to node")
var ErrUnableToCastMeta = errors.New("unable to cast interface to object")
var ErrMissingNodeAutoDiscoveryLabel = errors.New("missing node-auto-discovery label")
var ErrMissingTenantLabel = errors.New("missing tenant label")
var ErrUnableToCastShard = errors.New("unable to cast interface to ShuffleShard")
var ErrShuffleShardUpdated = errors.New("ShuffleShard update received")
