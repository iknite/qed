package hyper

import (
	"bytes"
	"math"
	"sync"

	"github.com/bbva/qed/balloon/common"
	"github.com/bbva/qed/db"
	"github.com/bbva/qed/log"
	"github.com/bbva/qed/util"
)

type HyperTree struct {
	lock          sync.RWMutex
	store         db.Store
	cache         common.ModifiableCache
	hasherF       func() common.Hasher
	cacheLevel    uint16
	defaultHashes []common.Digest
	hasher        common.Hasher
}

func NewHyperTree(hasherF func() common.Hasher, store db.Store, cache common.ModifiableCache) *HyperTree {
	var lock sync.RWMutex
	hasher := hasherF()
	cacheLevel := hasher.Len() - uint16(math.Max(float64(2), math.Floor(float64(hasher.Len())/10)))
	tree := &HyperTree{
		lock:          lock,
		store:         store,
		cache:         cache,
		hasherF:       hasherF,
		cacheLevel:    cacheLevel,
		defaultHashes: make([]common.Digest, hasher.Len()),
		hasher:        hasher,
	}

	tree.defaultHashes[0] = tree.hasher.Do([]byte{0x0}, []byte{0x0})
	for i := uint16(1); i < hasher.Len(); i++ {
		tree.defaultHashes[i] = tree.hasher.Do(tree.defaultHashes[i-1], tree.defaultHashes[i-1])
	}
	return tree
}

func (t *HyperTree) Add(eventDigest common.Digest, version uint64) (common.Digest, []db.Mutation, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	log.Debugf("Adding event %b with version %d\n", eventDigest, version)

	// visitors
	computeHash := common.NewComputeHashVisitor(t.hasher)
	caching := common.NewCachingVisitor(computeHash)

	// build pruning context
	versionAsBytes := util.Uint64AsBytes(version)
	context := PruningContext{
		navigator:     NewHyperTreeNavigator(t.hasher.Len()),
		cacheResolver: NewSingleTargetedCacheResolver(t.hasher.Len(), t.cacheLevel, eventDigest),
		cache:         t.cache,
		store:         t.store,
		defaultHashes: t.defaultHashes,
	}

	// traverse from root and generate a visitable pruned tree
	pruned := NewInsertPruner(eventDigest, versionAsBytes, context).Prune()

	// print := common.NewPrintVisitor(t.hasher.Len())
	// pruned.PreOrder(print)
	// log.Debugf("Pruned tree: %s", print.Result())

	// visit the pruned tree
	rootHash := pruned.PostOrder(caching).(common.Digest)

	// persist mutations
	cachedElements := caching.Result()
	mutations := make([]db.Mutation, len(cachedElements))
	for i, e := range cachedElements {
		mutations[i] = *db.NewMutation(db.HyperCachePrefix, e.Pos.Bytes(), e.Digest)
		// update cache
		t.cache.Put(e.Pos, e.Digest)
	}
	// create a mutation for the new leaf
	leafMutation := db.NewMutation(db.IndexPrefix, eventDigest, versionAsBytes)
	mutations = append(mutations, *leafMutation)

	log.Debugf("Mutations: %v", mutations)

	return rootHash, mutations, nil
}

func (t *HyperTree) QueryMembership(eventDigest common.Digest) (proof *QueryProof, err error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	log.Debugf("Getting version for event %b\n", eventDigest)

	pair, err := t.store.Get(db.IndexPrefix, eventDigest) // TODO check existence
	if err != nil {
		return nil, err
	}

	// visitors
	computeHash := common.NewComputeHashVisitor(t.hasher)
	calcAuditPath := common.NewAuditPathVisitor(computeHash)

	// build pruning context
	context := PruningContext{
		navigator:     NewHyperTreeNavigator(t.hasher.Len()),
		cacheResolver: NewSingleTargetedCacheResolver(t.hasher.Len(), t.cacheLevel, eventDigest),
		cache:         t.cache,
		store:         t.store,
		defaultHashes: t.defaultHashes,
	}

	// traverse from root and generate a visitable pruned tree
	pruned := NewSearchPruner(eventDigest, context).Prune()

	print := common.NewPrintVisitor(t.hasher.Len())
	pruned.PreOrder(print)
	log.Debugf("Pruned tree: %s", print.Result())

	// visit the pruned tree
	pruned.PostOrder(calcAuditPath)

	return NewQueryProof(pair.Key, pair.Value, calcAuditPath.Result(), t.hasherF()), nil // TODO include version in audit path visitor
}

func (t *HyperTree) VerifyMembership(proof *QueryProof, version uint64, eventDigest, expectedDigest common.Digest) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	log.Debugf("Verifying membership for eventDigest %x", eventDigest)

	// visitors
	computeHash := common.NewComputeHashVisitor(t.hasher)

	// build pruning context
	versionAsBytes := util.Uint64AsBytes(version)
	context := PruningContext{
		navigator:     NewHyperTreeNavigator(t.hasher.Len()),
		cacheResolver: NewSingleTargetedCacheResolver(t.hasher.Len(), t.cacheLevel, eventDigest),
		cache:         proof.AuditPath(),
		store:         t.store,
		defaultHashes: t.defaultHashes,
	}

	// traverse from root and generate a visitable pruned tree
	pruned := NewVerifyPruner(eventDigest, versionAsBytes, context).Prune()

	print := common.NewPrintVisitor(t.hasher.Len())
	pruned.PreOrder(print)
	log.Debugf("Pruned tree: %s", print.Result())

	// visit the pruned tree
	recomputed := pruned.PostOrder(computeHash).(common.Digest)
	return bytes.Equal(recomputed, expectedDigest)
}

func (t *HyperTree) Close() {
	t.cache = nil
	t.hasher = nil
	t.defaultHashes = nil
	t.store = nil
}
