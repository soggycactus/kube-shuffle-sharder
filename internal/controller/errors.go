package controller

import "errors"

var ErrUnableToCastNode = errors.New("unable to cast interface to node")
var ErrUnableToCastMeta = errors.New("unable to cast interface to object")
var ErrMissingNodeAutoDiscoveryLabel = errors.New("missing node-auto-discovery label")
