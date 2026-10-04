package engine

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// A request bound from the URL query string has no body and no header parameter,
// and a response type that writes raw bytes is documented with its media types.
func TestQueryStringRequestAndBinaryResponseContract(t *testing.T) {
	withSettings(t, Settings{
		QueryString: map[string][]QueryField{
			"example.assets.protocol.GetAssetReq": {{Name: "ref", Required: true, Description: "Storage reference of the asset"}},
		},
		BinaryResponses: map[string]BinaryResponse{
			"example.assets.protocol.GetAssetResp": {
				Description:  "Public image",
				ContentTypes: []string{"image/png", "image/jpeg", "image/webp"},
				Errors:       map[string]string{"404": "Asset not found", "503": "Asset storage unavailable"},
			},
		},
	}, "example")
	doc := &openapi3.T{Components: &openapi3.Components{}}
	method := &Method{
		APIPath: "/api/assets/getAsset",
		Doc:     "GetAsset serves a public image.\n@method: GET",
		Params: []ParamSpec{{In: "body", Name: "req", Types: []*TypeDescriptor{
			{FullKey: "example.assets.protocol.GetAssetReq"},
		}}, {In: "header", Name: "Headers", Required: true, Types: []*TypeDescriptor{
			{FullKey: "example.protocol.BaseHeader"},
		}}},
		Responses: []ResponseSpec{{Code: "200", DataType: &TypeDescriptor{
			FullKey: "example.assets.protocol.GetAssetResp",
		}}},
	}
	op := buildOperation(method, doc)
	require.Nil(t, op.RequestBody, "GET must not require a JSON body")
	require.Len(t, op.Parameters, 1)
	param := op.Parameters[0].Value
	require.Equal(t, "query", param.In)
	require.Equal(t, "ref", param.Name)
	require.True(t, param.Required)
	require.True(t, param.Schema.Value.Type.Is("string"))
	require.NotNil(t, op.Responses.Value("404"))
	require.NotNil(t, op.Responses.Value("503"))
	content := op.Responses.Value("200").Value.Content
	require.Len(t, content, 3)
	for _, mime := range []string{"image/png", "image/jpeg", "image/webp"} {
		require.Contains(t, content, mime)
		schema := content[mime].Schema.Value
		require.True(t, schema.Type.Is("string"))
		require.Equal(t, "binary", schema.Format)
		require.Empty(t, schema.Properties, "raw bytes must not advertise a JSON envelope")
	}
}
