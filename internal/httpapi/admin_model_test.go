package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleListModels_IncludesModeGroupsAndQuotaSync(t *testing.T) {
	disabled := false
	reg := registry.NewTestRegistry(
		[]modelconfig.ModelSpec{
			{
				ID:          "grok-4.20",
				DisplayName: "Grok 4.20",
				Type:        modelconfig.TypeChat,
				Enabled:     true,
				PoolFloor:   modelconfig.PoolBasic,
				Mode:        "auto",
				PublicType:  "chat",
			},
			{
				ID:              "grok-imagine-image",
				DisplayName:     "Grok Imagine Image",
				Type:            modelconfig.TypeImageWS,
				Enabled:         true,
				PoolFloor:       modelconfig.PoolSuper,
				QuotaSync:       &disabled,
				CooldownSeconds: 300,
				PublicType:      "image_ws",
			},
		},
		[]modelconfig.ModeSpec{
			{
				ID:            "auto",
				UpstreamName:  "auto",
				WindowSeconds: 7200,
				DefaultQuota: map[string]int{
					"basic": 20,
					"super": 50,
					"heavy": 150,
				},
			},
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	rec := httptest.NewRecorder()
	handleListModels(reg).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp modelCatalogResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Models, 2)
	require.Len(t, resp.ModeGroups, 1)

	assert.Equal(t, "auto", resp.ModeGroups[0].Mode)
	assert.Equal(t, "auto", resp.ModeGroups[0].DisplayName)
	assert.Equal(t, "auto", resp.ModeGroups[0].UpstreamName)
	assert.Equal(t, []string{"grok-4.20"}, resp.ModeGroups[0].Models)

	var imageWS *modelCatalogEntry
	for i := range resp.Models {
		if resp.Models[i].ID == "grok-imagine-image" {
			imageWS = &resp.Models[i]
			break
		}
	}
	require.NotNil(t, imageWS)
	assert.False(t, imageWS.QuotaSync)
	assert.Equal(t, 300, imageWS.CooldownSeconds)
}
