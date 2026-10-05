package pipeline

import (
	"path/filepath"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"

	"github.com/getkin/kin-openapi/openapi3"
)

// moduleLoader loads the module that is documented once, when the first document
// needs it, for every document of a run: loading and type-checking it is most of
// what a run costs, and what it gives is only read.
type moduleLoader struct {
	module *engine.Module
}

func (l *moduleLoader) get() (*engine.Module, error) {
	if l.module == nil {
		module, err := engine.LoadModule(rootDir)
		if err != nil {
			return nil, err
		}
		l.module = module
	}
	return l.module, nil
}

// buildDoc generates one document as its configuration describes it.
func buildDoc(d config.Doc, cfg *config.Config, first bool, loader *moduleLoader) error {
	overlays, err := loadOverlays(rootDir, d.Overlay)
	if err != nil {
		return err
	}
	audience := engine.Audience{Name: d.Audience, LegacyFieldTokens: d.LegacyFieldTokens, HiddenTypePrefixes: d.HideTypePrefixes}
	module, err := loader.get()
	if err != nil {
		return err
	}
	session, err := engine.NewGenerationSession(rootDir, engine.WithAudience(audience), engine.WithModule(module))
	if err != nil {
		return err
	}
	defer session.Close()
	hides = session.Hides
	defer func() { hides = nil }()

	var servers openapi3.Servers
	for _, s := range d.Servers {
		servers = append(servers, &openapi3.Server{URL: s.URL, Description: s.Description})
	}
	// initialize the document
	doc := &openapi3.T{
		OpenAPI: "3.0.0", // kin-openapi reads and writes 3.0 only
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
	if first {
		warnAboutTypeKeysThatNameNoType(cfg, session)
		warnAboutGenericTitlesThatDoNothing(cfg)
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
	if !legacyOutput {
		warnAboutPlaceholders(d, doc)
	}

	output := filepath.Join(outDir, d.Output)
	if d.Audience == config.AudiencePublic {
		warnAboutForceKeepsThatNameNoSchema(d, doc)
		return generatePublicYAML(doc, output, cfg.GenericTitles, d.ForceKeep, publicSettings(d.Public))
	}

	// generate the document
	return generateInternalYAML(doc, output, cfg.GenericTitles)
}
