package pipeline

import (
	"path/filepath"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"

	"github.com/getkin/kin-openapi/openapi3"
)

// buildDoc generates one document as its configuration describes it.
func buildDoc(d config.Doc, genericTitles []string) error {
	overlays, err := loadOverlays(rootDir, d.Overlay)
	if err != nil {
		return err
	}
	audience := engine.Audience{Name: d.Audience, LegacyFieldTokens: d.LegacyFieldTokens, HiddenTypePrefixes: d.HideTypePrefixes}
	session, err := engine.NewGenerationSession(rootDir, engine.WithAudience(audience))
	if err != nil {
		return err
	}
	defer session.Close()

	var servers openapi3.Servers
	for _, s := range d.Servers {
		servers = append(servers, &openapi3.Server{URL: s.URL, Description: s.Description})
	}
	// initialize the document
	doc := &openapi3.T{
		OpenAPI: "3.0.0", // i want 3.1.0
		Info: &openapi3.Info{
			Title:       d.Info.Title,
			Version:     d.Info.Version,
			Description: d.Info.Description,
		},
		Paths:   &openapi3.Paths{},
		Servers: servers,
		Components: &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		},
	}
	warnAboutPatternsThatMatchNothing(d, session.Packages())
	if err := GenerateModels(doc, d.Models, nil, session); err != nil {
		return err
	}
	applyOverlays(doc, overlays, config.StageAfterModels)
	if err := GenerateAPIs(doc, d.Services, session); err != nil {
		return err
	}
	if doc.Paths.Len() == 0 {
		engine.Logger().Warn("the document has no operations: no method of the packages that match \"services\" is an operation",
			"document", d.Name, "services", d.Services)
	}
	applyOverlays(doc, overlays, config.StageAfterAPIs)

	output := filepath.Join(outDir, d.Output)
	if d.Audience == config.AudiencePublic {
		return generatePublicYAML(doc, output, genericTitles, d.ForceKeep, publicSettings(d.Public))
	}

	// generate the document
	return generateInternalYAML(doc, output, genericTitles)
}
