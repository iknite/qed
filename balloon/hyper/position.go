// Copyright © 2018 Banco Bilbao Vizcaya Argentaria S.A.  All rights reserved.
// Use of this source code is governed by an Apache 2 License
// that can be found in the LICENSE file

package hyper

import (
	"encoding/binary"
	"fmt"
)

// A position identifies a unique node in the tree by its base, split and height
type Position struct {
	base   []byte // the left-most leaf in this node subtree
	split  []byte // the left-most leaf in the right branch of this node subtree
	height int    // height in the tree of this node
	n      int    // number of bits in the hash key
}

// returns a string representation of the position
func (p Position) String() string {
	return fmt.Sprintf("base: %b , split: %b , height: %d , n: %d", p.base, p.split, p.height, uint(p.n))
}

func (p Position) len() int {
	return p.n / 8
}

// returns a new position pointing to the left child
func (p Position) left() *Position {
	var np Position
	np.base = p.base
	np.height = p.height - 1
	np.n = p.n

	np.split = make([]byte, p.len())
	copy(np.split, np.base)

	splitBit := int(np.n - np.height)
	if splitBit < np.n {
		bitSet(np.split, splitBit)
	}

	return &np
}

// returns a new position pointing to the right child
func (p Position) right() *Position {
	var np Position
	np.base = p.split
	np.height = p.height - 1
	np.n = p.n

	np.split = make([]byte, p.len())
	copy(np.split, np.base)

	splitBit := int(np.n - np.height)
	if splitBit < np.n {
		bitSet(np.split, splitBit)
	}

	return &np
}

func (p Position) heightBytes() []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(p.height))
	return bytes
}

// creates the tree root position
func rootPosition(n int) *Position {
	var p Position
	p.height = n
	p.n = n
	p.base = make([]byte, p.len())
	p.split = make([]byte, p.len())

	bitSet(p.split, 0)

	return &p
}

func bitIsSet(bits []byte, i int) bool { return bits[i/8]&(1<<uint(7-i%8)) != 0 }
func bitSet(bits []byte, i int)        { bits[i/8] |= 1 << uint(7-i%8) }
func bitUnset(bits []byte, i int)      { bits[i/8] &= 0 << uint(7-i%8) }
