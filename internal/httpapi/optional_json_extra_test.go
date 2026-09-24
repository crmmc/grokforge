package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type optionalPayload struct {
	U optionalUint   `json:"u"`
	S optionalString `json:"s"`
}

func TestOptionalUint_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantSet bool
		wantNil bool
		wantVal uint
	}{
		{name: "numeric value", input: `{"u":42}`, wantSet: true, wantVal: 42},
		{name: "zero value", input: `{"u":0}`, wantSet: true, wantVal: 0},
		{name: "null clears value", input: `{"u":null}`, wantSet: true, wantNil: true},
		{name: "invalid type", input: `{"u":"x"}`, wantErr: true},
		{name: "negative rejected", input: `{"u":-1}`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var p optionalPayload
			err := json.Unmarshal([]byte(tc.input), &p)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, p.U.Set)
			if tc.wantNil {
				assert.Nil(t, p.U.Value)
			} else {
				require.NotNil(t, p.U.Value)
				assert.Equal(t, tc.wantVal, *p.U.Value)
			}
		})
	}
}

func TestOptionalString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantSet bool
		wantNil bool
		wantVal string
	}{
		{name: "string value", input: `{"s":"hello"}`, wantSet: true, wantVal: "hello"},
		{name: "empty string", input: `{"s":""}`, wantSet: true, wantVal: ""},
		{name: "null clears value", input: `{"s":null}`, wantSet: true, wantNil: true},
		{name: "invalid type", input: `{"s":123}`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var p optionalPayload
			err := json.Unmarshal([]byte(tc.input), &p)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, p.S.Set)
			if tc.wantNil {
				assert.Nil(t, p.S.Value)
			} else {
				require.NotNil(t, p.S.Value)
				assert.Equal(t, tc.wantVal, *p.S.Value)
			}
		})
	}
}

func TestOptionalFields_AbsentMeansNotSet(t *testing.T) {
	var p optionalPayload
	require.NoError(t, json.Unmarshal([]byte(`{}`), &p))
	assert.False(t, p.U.Set)
	assert.False(t, p.S.Set)
	assert.Nil(t, p.U.Value)
	assert.Nil(t, p.S.Value)
}
