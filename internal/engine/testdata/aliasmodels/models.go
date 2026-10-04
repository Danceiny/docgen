package aliasmodels

import aliasexternal "github.com/Danceiny/docgen/internal/engine/testdata/aliasexternal"

type Collision = aliasexternal.Collision

type Envelope struct {
	Value Collision `json:"value"`
}
