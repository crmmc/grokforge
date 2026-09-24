package store

import (
	"database/sql/driver"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringSliceValue(t *testing.T) {
	tests := []struct {
		name  string
		slice StringSlice
		want  driver.Value
	}{
		{name: "nil encodes to empty array", slice: nil, want: "[]"},
		{name: "empty encodes to empty array", slice: StringSlice{}, want: "[]"},
		{name: "values encode as json", slice: StringSlice{"a", "b"}, want: `["a","b"]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.slice.Value()
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStringSliceScan(t *testing.T) {
	tests := []struct {
		name    string
		src     any
		want    StringSlice
		wantErr bool
	}{
		{name: "nil becomes nil slice", src: nil, want: nil},
		{name: "string source", src: `["a","b"]`, want: StringSlice{"a", "b"}},
		{name: "byte source", src: []byte(`["c"]`), want: StringSlice{"c"}},
		{name: "invalid json", src: `["a", 1]`, wantErr: true},
		{name: "unsupported source type", src: 42, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got StringSlice
			err := got.Scan(tc.src)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIntMapValue(t *testing.T) {
	tests := []struct {
		name string
		m    IntMap
		want driver.Value
	}{
		{name: "nil encodes to empty object", m: nil, want: "{}"},
		{name: "empty encodes to empty object", m: IntMap{}, want: "{}"},
		{name: "values encode as json", m: IntMap{"auto": 5}, want: `{"auto":5}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.m.Value()
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIntMapScan(t *testing.T) {
	tests := []struct {
		name    string
		src     any
		want    IntMap
		wantErr bool
	}{
		{name: "nil becomes nil map", src: nil, want: nil},
		{name: "string source", src: `{"auto":5}`, want: IntMap{"auto": 5}},
		{name: "byte source", src: []byte(`{"auto":0}`), want: IntMap{"auto": 0}},
		{name: "invalid json", src: `{"auto":"x"}`, wantErr: true},
		{name: "unsupported source type", src: 3.14, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got IntMap
			err := got.Scan(tc.src)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCustomTypesRoundTripThroughDB(t *testing.T) {
	db := setupAPIKeyTestDB(t)

	lastUsed := time.Now().UTC().Truncate(time.Second)
	ak := &APIKey{
		Name:           "types-roundtrip",
		ModelWhitelist: StringSlice{"m1", "m2"},
	}
	require.NoError(t, db.Create(ak).Error)

	token := &Token{
		Token:       "types-roundtrip-token",
		Pool:        "ssoBasic",
		Status:      TokenStatusActive,
		Quotas:      IntMap{"auto": 10},
		LimitQuotas: IntMap{"auto": 100},
		ResumeAts:   IntMap{"auto": 1700000000},
		LastUsed:    &lastUsed,
	}
	require.NoError(t, db.Create(token).Error)

	var (
		foundKey   APIKey
		foundToken Token
	)
	require.NoError(t, db.First(&foundKey, ak.ID).Error)
	assert.Equal(t, StringSlice{"m1", "m2"}, foundKey.ModelWhitelist)

	require.NoError(t, db.First(&foundToken, token.ID).Error)
	assert.Equal(t, IntMap{"auto": 10}, foundToken.Quotas)
	assert.Equal(t, IntMap{"auto": 100}, foundToken.LimitQuotas)
	assert.Equal(t, IntMap{"auto": 1700000000}, foundToken.ResumeAts)
	require.NotNil(t, foundToken.LastUsed)
	assert.True(t, lastUsed.Equal(*foundToken.LastUsed))
}

func TestAutoMigrate_ErrorOnClosedDB(t *testing.T) {
	assert.Error(t, AutoMigrate(closedDB(t)))
}
