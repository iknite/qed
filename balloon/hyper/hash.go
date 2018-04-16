package hyper

import (
	"bytes"
	"verifiabledata/balloon/hashing"
)

// Constant Empty is a constant for empty leaves
var Empty = []byte{0x00}

// Constant Set is a constant for non-empty leaves
var Set = []byte{0x01}

type LeafHasher func([]byte, []byte, []byte) []byte
type InteriorHasher func([]byte, []byte, []byte, []byte) []byte

func leafHash(hasher hashing.Hasher, id, a, base []byte) []byte {
	if bytes.Equal(a, Empty) {
		return hasher(id)
	}
	return hasher(id, base)
}

func interiorHash(hasher hashing.Hasher, left, right, base, height []byte) []byte {
	if bytes.Equal(left, right) {
		return hasher(left, right)
	}
	return hasher(left, right, base, height)
}

func leafH(hasher hashing.Hasher) LeafHasher {
	return func(id, a, base []byte) []byte {
		if bytes.Equal(a, Empty) {
			return hasher(id)
		}
		return hasher(id, base)
	}
}

func interiorH(hasher hashing.Hasher) InteriorHasher {
	return func(left, right, base, height []byte) []byte {
		if bytes.Equal(left, right) {
			return hasher(left, right)
		}
		return hasher(left, right, base, height)
	}
}
