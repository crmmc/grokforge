package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type configJSONPayload struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestConfigStore_SetGetDelete(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewConfigStore(db)

	// Missing key returns empty value without error.
	value, err := s.Get("missing")
	require.NoError(t, err)
	assert.Empty(t, value)

	require.NoError(t, s.Set("k1", "v1"))
	value, err = s.Get("k1")
	require.NoError(t, err)
	assert.Equal(t, "v1", value)

	// Upsert overwrites the previous value.
	require.NoError(t, s.Set("k1", "v2"))
	value, err = s.Get("k1")
	require.NoError(t, err)
	assert.Equal(t, "v2", value)

	all, err := s.GetAll()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"k1": "v2"}, all)

	require.NoError(t, s.Delete("k1"))
	value, err = s.Get("k1")
	require.NoError(t, err)
	assert.Empty(t, value, "deleted key must read back as empty")

	all, err = s.GetAll()
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestConfigStore_SetMany(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewConfigStore(db)

	require.NoError(t, s.SetMany(map[string]string{"a": "1", "b": "2"}))
	require.NoError(t, s.SetMany(map[string]string{"a": "11", "c": "3"}))

	all, err := s.GetAll()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "11", "b": "2", "c": "3"}, all)

	// Empty map is a no-op.
	require.NoError(t, s.SetMany(map[string]string{}))
	all, err = s.GetAll()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "11", "b": "2", "c": "3"}, all)
}

func TestConfigStore_JSON(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, s *ConfigStore)
	}{
		{
			name: "round trip",
			run: func(t *testing.T, s *ConfigStore) {
				require.NoError(t, s.SetJSON("cfg", configJSONPayload{Name: "x", N: 7}))
				var got configJSONPayload
				require.NoError(t, s.GetJSON("cfg", &got))
				assert.Equal(t, configJSONPayload{Name: "x", N: 7}, got)
			},
		},
		{
			name: "missing key leaves destination untouched",
			run: func(t *testing.T, s *ConfigStore) {
				dest := configJSONPayload{Name: "keep"}
				require.NoError(t, s.GetJSON("nope", &dest))
				assert.Equal(t, "keep", dest.Name)
			},
		},
		{
			name: "stored non-json value fails to unmarshal",
			run: func(t *testing.T, s *ConfigStore) {
				require.NoError(t, s.Set("bad", "not-json"))
				var got configJSONPayload
				assert.Error(t, s.GetJSON("bad", &got))
			},
		},
		{
			name: "unmarshalable value fails to marshal",
			run: func(t *testing.T, s *ConfigStore) {
				assert.Error(t, s.SetJSON("chan", make(chan int)))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewConfigStore(setupAPIKeyTestDB(t))
			tc.run(t, s)
		})
	}
}

func TestConfigStore_SetMany_RollsBackOnError(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewConfigStore(db)
	failOnNthProcessor(t, db, "create", 2) // second upsert inside the tx fails

	err := s.SetMany(map[string]string{"a": "1", "b": "2"})
	assert.ErrorIs(t, err, errInjected)

	all, err := s.GetAll()
	require.NoError(t, err)
	assert.Empty(t, all, "failed transaction must roll back both entries")
}

func TestConfigStore_QueryErrorPaths(t *testing.T) {
	tests := []struct {
		name string
		act  func(s *ConfigStore) error
	}{
		{
			name: "get fails",
			act:  func(s *ConfigStore) error { _, err := s.Get("k"); return err },
		},
		{
			name: "set fails",
			act:  func(s *ConfigStore) error { return s.Set("k", "v") },
		},
		{
			name: "get all fails",
			act:  func(s *ConfigStore) error { _, err := s.GetAll(); return err },
		},
		{
			name: "set many fails",
			act:  func(s *ConfigStore) error { return s.SetMany(map[string]string{"k": "v"}) },
		},
		{
			name: "delete fails",
			act:  func(s *ConfigStore) error { return s.Delete("k") },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewConfigStore(closedDB(t))
			assert.Error(t, tc.act(s))
		})
	}
}

func TestConfigStore_GetWrapsKeyInError(t *testing.T) {
	s := NewConfigStore(closedDB(t))
	_, err := s.Get("the-key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `config get "the-key"`)
}

func TestConfigStore_GetJSONPropagatesGetError(t *testing.T) {
	s := NewConfigStore(closedDB(t))
	var dest configJSONPayload
	err := s.GetJSON("k", &dest)
	assert.Error(t, err)
}
