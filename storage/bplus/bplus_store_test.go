/*
   Copyright 2018-2019 Banco Bilbao Vizcaya Argentaria, S.A.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package bplus

import (
	"testing"

	"github.com/bbva/qed/storage"
	"github.com/bbva/qed/testutils/rand"
	"github.com/bbva/qed/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutate(t *testing.T) {
	store, closeF := openBPlusTreeStore()
	defer closeF()

	tests := []struct {
		testname      string
		table         storage.Table
		key, value    []byte
		expectedError error
	}{
		{"Mutate Key=Value", storage.HistoryCacheTable, []byte("Key"), []byte("Value"), nil},
	}

	for _, test := range tests {
		err := store.Mutate([]*storage.Mutation{
			{
				Table: test.table,
				Key:   test.key,
				Value: test.value,
			},
		})
		require.Equalf(t, test.expectedError, err, "Error mutating in test: %s", test.testname)
		_, err = store.Get(test.table, test.key)
		require.Equalf(t, test.expectedError, err, "Error getting key in test: %s", test.testname)
	}
}

func TestGetExistentKey(t *testing.T) {

	store, closeF := openBPlusTreeStore()
	defer closeF()

	testCases := []struct {
		table         storage.Table
		key, value    []byte
		expectedError error
	}{
		{storage.HistoryCacheTable, []byte("Key1"), []byte("Value1"), nil},
		{storage.HistoryCacheTable, []byte("Key2"), []byte("Value2"), nil},
		{storage.HyperCacheTable, []byte("Key3"), []byte("Value3"), nil},
		{storage.HyperCacheTable, []byte("Key4"), []byte("Value4"), storage.ErrKeyNotFound},
	}

	for _, test := range testCases {
		if test.expectedError == nil {
			err := store.Mutate([]*storage.Mutation{
				{test.table, test.key, test.value},
			})
			require.NoError(t, err)
		}

		stored, err := store.Get(test.table, test.key)
		if test.expectedError == nil {
			require.NoError(t, err)
			require.Equalf(t, stored.Key, test.key, "The stored key does not match the original: expected %d, actual %d", test.key, stored.Key)
			require.Equalf(t, stored.Value, test.value, "The stored value does not match the original: expected %d, actual %d", test.value, stored.Value)
		} else {
			require.Error(t, test.expectedError)
		}
	}
}

func TestGetRange(t *testing.T) {
	store, closeF := openBPlusTreeStore()
	defer closeF()

	var testCases = []struct {
		size       int
		start, end byte
	}{
		{40, 10, 50},
		{0, 1, 9},
		{11, 1, 20},
		{10, 40, 60},
		{0, 60, 100},
		{0, 20, 10},
	}

	table := storage.HistoryCacheTable
	for i := 10; i < 50; i++ {
		store.Mutate([]*storage.Mutation{
			{table, []byte{byte(i)}, []byte("Value")},
		})
	}

	for _, test := range testCases {
		slice, err := store.GetRange(table, []byte{test.start}, []byte{test.end})
		require.NoError(t, err)
		require.Equalf(t, len(slice), test.size, "Slice length invalid: expected %d, actual %d", test.size, len(slice))
	}

}

func TestGetAll(t *testing.T) {

	table := storage.HyperCacheTable
	numElems := uint16(1000)
	testCases := []struct {
		batchSize    int
		numBatches   int
		lastBatchLen int
	}{
		{10, 100, 10},
		{20, 50, 20},
		{17, 59, 14},
	}

	store, closeF := openBPlusTreeStore()
	defer closeF()

	// insert
	for i := uint16(0); i < numElems; i++ {
		key := util.Uint16AsBytes(i)
		store.Mutate([]*storage.Mutation{
			{table, key, key},
		})
	}

	for i, c := range testCases {
		reader := store.GetAll(table)
		numBatches := 0
		var lastBatchLen int
		for {
			entries := make([]*storage.KVPair, c.batchSize)
			n, _ := reader.Read(entries)
			if n == 0 {
				break
			}
			numBatches++
			lastBatchLen = n
		}
		reader.Close()
		assert.Equalf(t, c.numBatches, numBatches, "The number of batches should match for test case %d", i)
		assert.Equal(t, c.lastBatchLen, lastBatchLen, "The size of the last batch len should match for test case %d", i)
	}

}

func TestGetLast(t *testing.T) {
	store, closeF := openBPlusTreeStore()
	defer closeF()

	// insert
	numElems := uint64(20)
	tables := []storage.Table{storage.HistoryCacheTable, storage.HyperCacheTable}
	for _, table := range tables {
		for i := uint64(0); i < numElems; i++ {
			key := util.Uint64AsBytes(i)
			store.Mutate([]*storage.Mutation{
				{table, key, key},
			})
		}
	}

	// get last element for history table
	kv, err := store.GetLast(storage.HistoryCacheTable)
	require.NoError(t, err)
	require.Equalf(t, util.Uint64AsBytes(numElems-1), kv.Key, "The key should match the last inserted element")
	require.Equalf(t, util.Uint64AsBytes(numElems-1), kv.Value, "The value should match the last inserted element")
}

func BenchmarkMutate(b *testing.B) {
	store, closeF := openBPlusTreeStore()
	defer closeF()
	b.N = 10000
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Mutate([]*storage.Mutation{
			{storage.HistoryCacheTable, rand.Bytes(128), []byte("Value")},
		})
	}
}

func BenchmarkGet(b *testing.B) {
	store, closeF := openBPlusTreeStore()
	defer closeF()
	N := 10000
	b.N = N
	var key []byte

	// populate storage
	for i := 0; i < N; i++ {
		if i == 10 {
			key = rand.Bytes(128)
			store.Mutate([]*storage.Mutation{
				{storage.HistoryCacheTable, key, []byte("Value")},
			})
		} else {
			store.Mutate([]*storage.Mutation{
				{storage.HistoryCacheTable, rand.Bytes(128), []byte("Value")},
			})
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		store.Get(storage.HistoryCacheTable, key)
	}

}

func BenchmarkGetRangeInLargeTree(b *testing.B) {
	store, closeF := openBPlusTreeStore()
	defer closeF()
	N := 1000000

	// populate storage
	for i := 0; i < N; i++ {
		store.Mutate([]*storage.Mutation{
			{storage.HistoryCacheTable, []byte{byte(i)}, []byte("Value")},
		})
	}

	b.ResetTimer()

	b.Run("Small range", func(b *testing.B) {
		b.N = 10000
		for i := 0; i < b.N; i++ {
			store.GetRange(storage.HistoryCacheTable, []byte{10}, []byte{10})
		}
	})

	b.Run("Large range", func(b *testing.B) {
		b.N = 10000
		for i := 0; i < b.N; i++ {
			store.GetRange(storage.HistoryCacheTable, []byte{10}, []byte{35})
		}
	})

}

func openBPlusTreeStore() (*BPlusTreeStore, func()) {
	store := NewBPlusTreeStore()
	return store, func() {
		store.Close()
	}
}
