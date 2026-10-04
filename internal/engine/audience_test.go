package engine

import "testing"

// The audiences the visibility tests use. Each lists the apidoc:"<value>"
// tokens that show a field to it, the way a configuration does.
var (
	testInternal = Audience{Name: AudienceInternal, LegacyFieldTokens: []string{"Staff"}}
	testPublic   = Audience{Name: AudiencePublic, LegacyFieldTokens: []string{"Partner"}}
)

func TestAudienceMatchesOnlyItsOwnName(t *testing.T) {
	if !testInternal.isInternal() || testInternal.isPublic() {
		t.Fatal("internal audience must be internal only")
	}
	if !testPublic.isPublic() || testPublic.isInternal() {
		t.Fatal("public audience must be public only")
	}
	var neutral Audience
	if neutral.isPublic() || neutral.isInternal() {
		t.Fatal("the zero audience must match neither")
	}
}

func TestAudienceShowsOnlyListedFieldTokens(t *testing.T) {
	if !testInternal.listsFieldToken("Staff") {
		t.Fatal("a listed token must be shown")
	}
	// A token is matched whole: a part of a listed token is not listed.
	if testInternal.listsFieldToken("Sta") || testInternal.listsFieldToken("ff") {
		t.Fatal("only exact tokens are shown")
	}
	if testPublic.listsFieldToken("Staff") {
		t.Fatal("an audience shows only the tokens it lists")
	}
}

// Without a directive, a type is hidden from an audience when its name starts
// with one of the audience's hidden prefixes; with none, nothing is.
func TestTypesWithAHiddenPrefixAreHiddenByDefault(t *testing.T) {
	aud := Audience{Name: AudiencePublic, HiddenTypePrefixes: []string{"Internal", "Private"}}
	for name, want := range map[string]bool{
		"InternalAudit": true,
		"PrivateKey":    true,
		"Internals":     true, // a prefix is a prefix
		"MyInternal":    false,
		"Order":         false,
		"internalAudit": false, // names are case-sensitive
		"":              false,
	} {
		if got := shouldHideByDefault(name, aud); got != want {
			t.Errorf("shouldHideByDefault(%q) = %v, want %v", name, got, want)
		}
	}
	if shouldHideByDefault("InternalAudit", Audience{Name: AudiencePublic}) {
		t.Error("an audience without hidden prefixes hides nothing by default")
	}
	// The prefixes belong to the audience, not to its name: an internal document
	// may list some too.
	if !shouldHideByDefault("Secret", Audience{Name: AudienceInternal, HiddenTypePrefixes: []string{"Secret"}}) {
		t.Error("hidden prefixes apply to whatever audience lists them")
	}
}
