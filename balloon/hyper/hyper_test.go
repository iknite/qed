// Copyright © 2018 Banco Bilbao Vizcaya Argentaria S.A.  All rights reserved.
// Use of this source code is governed by an Apache 2 License
// that can be found in the LICENSE file

package hyper

import (
	"bytes"
	"verifiabledata/balloon/hashing"
)

func fakeLeafHasherF(hasher hashing.Hasher) LeafHasher {
	return func(id, value, base []byte) []byte {
		if bytes.Equal(value, Empty) {
			return hasher(Empty)
		}
		return hasher(base)
	}
}

func fakeInteriorHasherF(hasher hashing.Hasher) InteriorHasher {
	return func(left, right, base, height []byte) []byte {
		return hasher(left, right)
	}
}
