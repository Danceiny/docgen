package engine

import (
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildRequestBodyMultipartOptin: a request type that is configured as multipart must produce
// multipart/form-data with a binary file and scalar fields; other body endpoints stay JSON.
func TestBuildRequestBodyMultipartOptin(t *testing.T) {
	withSettings(t, uploadSettings(), "example")
	cases := []struct {
		name            string
		requestType     string
		wantContentType string
		wantFieldNames  []string
		wantFileFormat  string
		wantScalarTypes map[string]string
		wantRequired    []string
	}{
		{
			name:            "avatar upload",
			requestType:     "example.user.protocol.UploadAvatarReq",
			wantContentType: "multipart/form-data",
			wantFieldNames:  []string{"file", "kind", "ownerId"},
			wantFileFormat:  "binary",
			wantScalarTypes: map[string]string{"kind": "string", "ownerId": "integer"},
			wantRequired:    []string{"file", "kind"},
		},
		{
			name:            "image upload",
			requestType:     "example.content.protocol.UploadImageReq",
			wantContentType: "multipart/form-data",
			wantFieldNames:  []string{"file", "itemId", "groupId", "source"},
			wantFileFormat:  "binary",
			wantScalarTypes: map[string]string{"itemId": "integer", "groupId": "integer", "source": "string"},
			wantRequired:    []string{"file", "itemId"},
		},
		{
			name:            "logo upload",
			requestType:     "example.content.protocol.UploadLogoReq",
			wantContentType: "multipart/form-data",
			wantFieldNames:  []string{"file"},
			wantFileFormat:  "binary",
			wantRequired:    []string{"file"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method := &Method{
				APIPath: "/api/test/path",
				Params: []ParamSpec{{
					Name: "request", In: "body", Required: true,
					Types: []*TypeDescriptor{{FullKey: tc.requestType}},
				}},
			}
			body := buildRequestBody(method, &openapi3.T{})
			require.NotNil(t, body)
			media, ok := body.Value.Content[tc.wantContentType]
			require.True(t, ok, "expected content type %q, got keys %v", tc.wantContentType, keysOf(body.Value.Content))
			require.NotNil(t, media.Schema)
			props := media.Schema.Value.Properties
			require.ElementsMatch(t, tc.wantFieldNames, keysOf(props), "no internal upload metadata in wire schema")
			for _, fname := range tc.wantFieldNames {
				_, exists := props[fname]
				require.True(t, exists, "missing field %q in multipart schema; got keys %v", fname, keysOf(props))
				// required check: those listed in wantRequired must be there; those not listed must not be in
				// the required list (this matches the runtime meaning of optional scalar fields).
				if isRequired(t, tc.wantRequired, fname) {
					require.Contains(t, media.Schema.Value.Required, fname, "field %q must be required per allowlist", fname)
				} else {
					require.NotContains(t, media.Schema.Value.Required, fname, "field %q must not be required per allowlist", fname)
				}
			}
			if tc.wantFileFormat != "" {
				require.Equal(t, "binary", props["file"].Value.Format, "file field must be binary")
				require.NotNil(t, props["file"].Value.Type)
				require.Contains(t, *props["file"].Value.Type, "string", "file field must be string type")
			}
			require.ElementsMatch(t, tc.wantRequired, media.Schema.Value.Required, "required list must match allowlist + runtime semantics")
			for fname, wantType := range tc.wantScalarTypes {
				require.NotNil(t, props[fname].Value.Type, "scalar %q missing type", fname)
				require.Contains(t, *props[fname].Value.Type, wantType, "scalar %q type mismatch", fname)
				if wantType == "integer" {
					require.Equal(t, "int64", props[fname].Value.Format)
				}
			}
			// JSON content type must NOT be emitted alongside multipart (avoid contract noise).
			_, jsonPresent := body.Value.Content["application/json"]
			assert.False(t, jsonPresent, "multipart endpoint must not also advertise application/json")
		})
	}
}

// TestBuildRequestBodyJSONUnchanged: endpoints that are not multipart keep the application/json contract.
func TestBuildRequestBodyJSONUnchanged(t *testing.T) {
	withSettings(t, uploadSettings(), "example")
	method := &Method{
		APIPath: "/api/foo/bar",
		Params: []ParamSpec{{
			Name: "request", In: "body", Required: true,
			Types: []*TypeDescriptor{{FullKey: "example.foo.protocol.SomeJSONReq"}},
		}},
	}
	body := buildRequestBody(method, &openapi3.T{})
	require.NotNil(t, body)
	_, hasMultipart := body.Value.Content["multipart/form-data"]
	assert.False(t, hasMultipart, "non-multipart endpoint must not advertise multipart/form-data")
	_, hasJSON := body.Value.Content["application/json"]
	assert.True(t, hasJSON, "non-multipart endpoint must keep application/json")
}

// TestFirstBodyTypeKey covers the edge cases: a nil method, no body param, several body params and so on.
func TestFirstBodyTypeKey(t *testing.T) {
	assert.Equal(t, "", firstBodyTypeKey(nil))
	assert.Equal(t, "", firstBodyTypeKey(&Method{}))
	assert.Equal(t, "", firstBodyTypeKey(&Method{Params: []ParamSpec{{In: "header"}}}))
	// with several body params the first is taken (a method has one body in practice)
	got := firstBodyTypeKey(&Method{Params: []ParamSpec{
		{In: "body", Types: []*TypeDescriptor{{FullKey: "a.b.C"}}},
		{In: "body", Types: []*TypeDescriptor{{FullKey: "a.b.D"}}},
	}})
	assert.Equal(t, "a.b.C", got)
}

// TestBuildRequestBodyNoBodyStillNil: an endpoint with no body parameter at all still gets nil; the multipart
// path is not adapted by accident to head requests or list queries.
func TestBuildRequestBodyNoBodyStillNil(t *testing.T) {
	method := &Method{APIPath: "/api/foo/get", Params: nil}
	assert.Nil(t, buildRequestBody(method, &openapi3.T{}))
}

// TestIsMultipartRequest reports exactly the configured request types.
func TestIsMultipartRequest(t *testing.T) {
	withSettings(t, uploadSettings(), "example")
	for _, key := range []string{
		"example.user.protocol.UploadAvatarReq",
		"example.content.protocol.UploadImageReq",
		"example.content.protocol.UploadLogoReq",
	} {
		assert.True(t, IsMultipartRequest(key), "expected %q to be configured as multipart", key)
	}
	assert.False(t, IsMultipartRequest("example.foo.protocol.NotMultipartReq"))

	Configure(Settings{})
	assert.False(t, IsMultipartRequest("example.user.protocol.UploadAvatarReq"), "nothing is multipart without configuration")
}

// uploadSettings configures three multipart request types: a file with scalars,
// a file with more scalars, and a file alone.
func uploadSettings() Settings {
	return Settings{Multipart: map[string][]MultipartField{
		"example.user.protocol.UploadAvatarReq": {
			{Name: "file", Kind: "file", Required: true},
			{Name: "kind", Kind: "scalar", ScalarTo: "string", Required: true},
			{Name: "ownerId", Kind: "scalar", ScalarTo: "integer"},
		},
		"example.content.protocol.UploadImageReq": {
			{Name: "file", Kind: "file", Required: true},
			{Name: "itemId", Kind: "scalar", ScalarTo: "integer", Required: true},
			{Name: "groupId", Kind: "scalar", ScalarTo: "integer"},
			{Name: "source", Kind: "scalar", ScalarTo: "string"},
		},
		"example.content.protocol.UploadLogoReq": {
			{Name: "file", Kind: "file", Required: true},
		},
	}}
}

// keysOf returns the keys of a map in order, for failure messages (it does not depend on how a std map iterates).
func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isRequired reports whether name is in the expected required list.
func isRequired(t *testing.T, wantRequired []string, name string) bool {
	t.Helper()
	for _, n := range wantRequired {
		if n == name {
			return true
		}
	}
	return false
}
