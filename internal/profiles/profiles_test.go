package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"airspace-acars/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParse(t *testing.T, doc string) *Profile {
	t.Helper()
	prof, err := Parse([]byte(doc))
	require.NoError(t, err)
	return prof
}

// --- Matcher ---

func TestMatchLeafOperators(t *testing.T) {
	ctx := Context{
		AircraftName: "Fenix A320 IAE Lufthansa",
		AircraftType: "A20N",
		Simulator:    SimSimConnect,
		EngineCount:  2,
	}

	cases := []struct {
		name  string
		node  string
		match bool
	}{
		{"contains is case insensitive", `{"field":"aircraftName","op":"contains","value":"fenix"}`, true},
		{"contains respects case when asked", `{"field":"aircraftName","op":"contains","value":"fenix","caseSensitive":true}`, false},
		{"notContains", `{"field":"aircraftName","op":"notContains","value":"pmdg"}`, true},
		{"startsWith", `{"field":"aircraftName","op":"startsWith","value":"Fenix"}`, true},
		{"endsWith", `{"field":"aircraftName","op":"endsWith","value":"lufthansa"}`, true},
		{"equals on type", `{"field":"aircraftType","op":"equals","value":"a20n"}`, true},
		{"notEquals on type", `{"field":"aircraftType","op":"notEquals","value":"a20n"}`, false},
		{"matches regex", `{"field":"aircraftName","op":"matches","value":"a\\s*-?320"}`, true},
		{"in list", `{"field":"aircraftType","op":"in","value":["A320","A20N"]}`, true},
		{"notIn list", `{"field":"aircraftType","op":"notIn","value":["B738"]}`, true},
		{"numeric equals", `{"field":"engineCount","op":"equals","value":2}`, true},
		{"greaterThan", `{"field":"engineCount","op":"greaterThan","value":2}`, false},
		{"lessThan", `{"field":"engineCount","op":"lessThan","value":4}`, true},
		{"exists", `{"field":"aircraftName","op":"exists"}`, true},
		{"simulator equals", `{"field":"simulator","op":"equals","value":"simconnect"}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prof := mustParse(t, `{"id":"t","name":"t","match":`+tc.node+`}`)
			assert.Equal(t, tc.match, prof.Match.Eval(ctx))
		})
	}
}

func TestMatchGroupsCombineWithAndOrNot(t *testing.T) {
	prof := mustParse(t, `{
		"id": "t", "name": "t",
		"match": {
			"all": [
				{"field": "simulator", "op": "equals", "value": "simconnect"},
				{"any": [
					{"field": "aircraftName", "op": "contains", "value": "fenix"},
					{"field": "aircraftName", "op": "contains", "value": "fslabs"}
				]},
				{"not": {"field": "aircraftName", "op": "contains", "value": "beta"}}
			]
		}
	}`)

	assert.True(t, prof.Match.Eval(Context{AircraftName: "FSLabs A321-NEO", Simulator: SimSimConnect}))
	assert.True(t, prof.Match.Eval(Context{AircraftName: "Fenix A320", Simulator: SimSimConnect}))
	assert.False(t, prof.Match.Eval(Context{AircraftName: "Fenix A320 beta", Simulator: SimSimConnect}),
		"not clause should reject the beta title")
	assert.False(t, prof.Match.Eval(Context{AircraftName: "Fenix A320", Simulator: SimXPlane}),
		"simulator clause should reject X-Plane")
	assert.False(t, prof.Match.Eval(Context{AircraftName: "PMDG 737", Simulator: SimSimConnect}))

	assert.Equal(t, 4, prof.Match.Conditions())
}

func TestMatchNilSelectorMatchesEverything(t *testing.T) {
	prof := mustParse(t, `{"id":"t","name":"t"}`)
	assert.True(t, prof.Match.Eval(Context{}))
}

func TestValidateRejectsBadSelectors(t *testing.T) {
	_, err := Parse([]byte(`{"id":"t","name":"t","match":{"field":"tailNumber","op":"equals","value":"x"}}`))
	assert.ErrorContains(t, err, "unknown match field")

	_, err = Parse([]byte(`{"id":"t","name":"t","match":{"field":"aircraftName","op":"sounds-like","value":"x"}}`))
	assert.ErrorContains(t, err, "unknown match operator")

	_, err = Parse([]byte(`{"id":"t","name":"t","match":{"field":"aircraftName","op":"matches","value":"a("}}`))
	assert.ErrorContains(t, err, "invalid regular expression")

	_, err = Parse([]byte(`{"id":"t","name":"t","mash":{"autopilot.mastr":{"source":{"kind":"simvar","name":"X"}}}}`))
	assert.ErrorContains(t, err, "unknown data point")

	_, err = Parse([]byte(`{"id":"t","name":"t","mash":{"autopilot.master":{"source":{"kind":"telepathy","name":"X"}}}}`))
	assert.ErrorContains(t, err, "unknown source kind")

	_, err = Parse([]byte(`{"id":"t","name":"t","mash":{"autopilot.master":{"source":{"kind":"simvar","name":"X"},"transform":[{"op":"warp"}]}}}`))
	assert.ErrorContains(t, err, "unknown transform op")

	_, err = Parse([]byte(`{"id":"t","name":"t","mash":{"aircraft.type":{"source":{"kind":"simvar","name":"ATC MODEL"}}}}`))
	assert.ErrorContains(t, err, "can only be fed by a const source")
}

// --- Transforms ---

func TestTransformPipeline(t *testing.T) {
	cases := []struct {
		name  string
		steps string
		in    float64
		want  float64
	}{
		{"scale", `[{"op":"scale","value":100}]`, 0.35, 35},
		{"divide then round", `[{"op":"divide","value":1000},{"op":"round","value":3}]`, 118255, 118.255},
		{"offset", `[{"op":"offset","value":-1}]`, 5, 4},
		{"clamp high", `[{"op":"clamp","min":0,"max":100}]`, 140, 100},
		{"clamp low", `[{"op":"clamp","min":0,"max":100}]`, -3, 0},
		{"bool from non-zero", `[{"op":"bool"}]`, 2, 1},
		{"bool with threshold", `[{"op":"bool","value":2}]`, 1, 0},
		{"not", `[{"op":"not"}]`, 0, 1},
		{"gte", `[{"op":"gte","value":1900}]`, 2000, 1},
		{"gte below", `[{"op":"gte","value":1900}]`, 1200, 0},
		{"map hit", `[{"op":"map","map":{"0":0,"1":25,"2":50,"3":75,"4":100}}]`, 3, 75},
		{"map default", `[{"op":"map","map":{"0":0},"default":-1}]`, 9, -1},
		{"abs", `[{"op":"abs"}]`, -12, 12},
		{"chained", `[{"op":"scale","value":2},{"op":"offset","value":1},{"op":"gt","value":10}]`, 6, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prof := mustParse(t, `{"id":"t","name":"t","mash":{"attitude.ias":{"source":{"kind":"simvar","name":"X"},"transform":`+tc.steps+`}}}`)
			steps := prof.Mash["attitude.ias"].Bindings[0].Transform
			assert.InDelta(t, tc.want, applySteps(steps, tc.in), 1e-9)
		})
	}
}

// --- Resolution ---

const dualBindingProfile = `{
	"id": "dual", "name": "Dual",
	"match": {"field": "aircraftName", "op": "contains", "value": "test"},
	"mash": {
		"autopilot.master": [
			{"sim": "simconnect", "source": {"kind": "lvar", "name": "I_FCU_AP1"}, "transform": [{"op": "bool"}]},
			{"sim": "simconnect", "source": {"kind": "simvar", "name": "AUTOPILOT MASTER", "unit": "Bool"}, "transform": [{"op": "bool"}]}
		]
	}
}`

func TestResolveFallsBackToSupportedSource(t *testing.T) {
	prof := mustParse(t, dualBindingProfile)
	ctx := Context{AircraftName: "Test 320", Simulator: SimSimConnect}

	simOnly := Resolve([]*Profile{prof}, ctx, []SourceKind{SourceSimVar})
	require.Len(t, simOnly.Bindings, 1)
	require.Len(t, simOnly.Bindings[0].Sources, 1)
	assert.Equal(t, SourceSimVar, simOnly.Bindings[0].Sources[0].Kind)
	assert.Equal(t, "AUTOPILOT MASTER", simOnly.Bindings[0].Sources[0].Name)

	withLVars := Resolve([]*Profile{prof}, ctx, []SourceKind{SourceSimVar, SourceLVar})
	require.Len(t, withLVars.Bindings, 1)
	assert.Equal(t, SourceLVar, withLVars.Bindings[0].Sources[0].Kind, "the local variable wins when it can be read")
	assert.Equal(t, "I_FCU_AP1", withLVars.Bindings[0].Sources[0].Name)
}

func TestResolveSkipsBindingWithNoUsableSource(t *testing.T) {
	prof := mustParse(t, `{
		"id": "lvar-only", "name": "LVar only",
		"mash": {"apu.rpmPercent": {"sim": "simconnect", "source": {"kind": "lvar", "name": "A32NX_APU_N"}}}
	}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar})
	assert.Empty(t, plan.Bindings)
	assert.Equal(t, []string{"apu.rpmPercent"}, plan.Skipped)
}

func TestResolveBindingScopedToOtherSimulatorIsIgnored(t *testing.T) {
	prof := mustParse(t, `{
		"id": "xp", "name": "XP",
		"mash": {"radios.com1": {"sim": "xplane", "source": {"kind": "dataref", "name": "sim/x"}}}
	}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar, SourceDataRef})
	assert.Empty(t, plan.Bindings)
}

func TestResolveHigherPriorityOverridesSamePoint(t *testing.T) {
	base := mustParse(t, `{
		"id": "base", "name": "Base", "priority": 100,
		"mash": {"aircraft.type": {"source": {"kind": "const", "value": "A321"}}}
	}`)
	variant := mustParse(t, `{
		"id": "variant", "name": "Variant", "priority": 120,
		"mash": {"aircraft.type": {"source": {"kind": "const", "value": "A21N"}}}
	}`)

	plan := Resolve([]*Profile{variant, base}, Context{}, nil)
	require.Len(t, plan.Bindings, 1)
	assert.Equal(t, "variant", plan.Bindings[0].ProfileID)
	assert.Equal(t, "A21N", plan.Bindings[0].Sources[0].Value.Str)
	assert.Equal(t, []string{"base", "variant"}, plan.ProfileIDs(), "profiles apply from lowest to highest priority")
}

func TestResolveDeduplicatesVariables(t *testing.T) {
	prof := mustParse(t, `{
		"id": "dup", "name": "Dup",
		"mash": {
			"autopilot.heading": {"source": {"kind": "simvar", "name": "HDG", "unit": "degrees"}},
			"attitude.headingMag": {"source": {"kind": "simvar", "name": "HDG", "unit": "degrees"}},
			"aircraft.type": {"source": {"kind": "const", "value": "A320"}}
		}
	}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar})
	assert.Len(t, plan.Bindings, 3)
	require.Len(t, plan.Vars(), 1, "one variable is enough to feed both points; constants need none")
	assert.Equal(t, "HDG", plan.Vars()[0].Name)
}

func TestDisabledProfileNeverMatches(t *testing.T) {
	prof := mustParse(t, `{"id":"off","name":"Off","disabled":true,"mash":{"aircraft.type":{"source":{"kind":"const","value":"A320"}}}}`)
	assert.Empty(t, Resolve([]*Profile{prof}, Context{}, nil).Bindings)
}

// --- Apply ---

func TestApplyWritesMashedPoints(t *testing.T) {
	prof := mustParse(t, `{
		"id": "apply", "name": "Apply",
		"mash": {
			"radios.com1": {"source": {"kind": "simvar", "name": "COM", "unit": "kHz"}, "transform": [{"op": "divide", "value": 1000}, {"op": "round", "value": 3}]},
			"controls.flaps": {"source": {"kind": "simvar", "name": "FLAP"}, "transform": [{"op": "map", "map": {"0": 0, "1": 25, "2": 50, "3": 75, "4": 100}}]},
			"autopilot.master": {"source": {"kind": "simvar", "name": "AP"}, "transform": [{"op": "bool"}]},
			"aircraft.type": {"source": {"kind": "const", "value": "A21N"}}
		}
	}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar})

	fd := &domain.FlightData{}
	fd.Radios.Com1 = 118.25
	fd.AircraftType = "A321"
	plan.Apply(fd, map[string]float64{
		"simvar:COM:kHz": 121905,
		"simvar:FLAP:":   2,
		"simvar:AP:":     1,
	})

	assert.InDelta(t, 121.905, fd.Radios.Com1, 1e-9)
	assert.InDelta(t, 50, fd.Controls.Flaps, 1e-9)
	assert.True(t, fd.Autopilot.Master)
	assert.Equal(t, "A21N", fd.AircraftType)
}

func TestApplyLeavesPointAloneUntilItsVariableArrives(t *testing.T) {
	prof := mustParse(t, `{"id":"a","name":"A","mash":{"autopilot.master":{"source":{"kind":"simvar","name":"AP"}}}}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar})

	fd := &domain.FlightData{}
	fd.Autopilot.Master = true
	plan.Apply(fd, map[string]float64{})
	assert.True(t, fd.Autopilot.Master, "the adapter's own reading must survive until the profile variable is received")
}

func TestApplyReduceCombinesEveryReading(t *testing.T) {
	prof := mustParse(t, `{
		"id": "reduce", "name": "Reduce",
		"mash": {
			"autopilot.master": {
				"reduce": "or",
				"bindings": [
					{"source": {"kind": "simvar", "name": "AP1"}, "transform": [{"op": "bool"}]},
					{"source": {"kind": "simvar", "name": "AP2"}, "transform": [{"op": "bool"}]}
				]
			},
			"controls.gearDown": {
				"reduce": "and",
				"bindings": [
					{"source": {"kind": "simvar", "name": "GEAR_L"}, "transform": [{"op": "gte", "value": 1900}]},
					{"source": {"kind": "simvar", "name": "GEAR_R"}, "transform": [{"op": "gte", "value": 1900}]}
				]
			}
		}
	}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar})
	require.Len(t, plan.Vars(), 4)

	fd := &domain.FlightData{}
	plan.Apply(fd, map[string]float64{
		"simvar:AP1:": 0, "simvar:AP2:": 1,
		"simvar:GEAR_L:": 2000, "simvar:GEAR_R:": 1400,
	})
	assert.True(t, fd.Autopilot.Master, "AP2 alone engages the autopilot")
	assert.False(t, fd.Controls.GearDown, "one leg short of down is not gear down")

	plan.Apply(fd, map[string]float64{
		"simvar:AP1:": 0, "simvar:AP2:": 0,
		"simvar:GEAR_L:": 2000, "simvar:GEAR_R:": 2000,
	})
	assert.False(t, fd.Autopilot.Master)
	assert.True(t, fd.Controls.GearDown)
}

func TestApplyEngineCountSetsEveryExistsFlag(t *testing.T) {
	prof := mustParse(t, `{"id":"e","name":"E","mash":{"engines.count":{"source":{"kind":"simvar","name":"N"}}}}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar})

	fd := &domain.FlightData{}
	plan.Apply(fd, map[string]float64{"simvar:N:": 3})
	assert.Equal(t, []bool{true, true, true, false},
		[]bool{fd.Engines[0].Exists, fd.Engines[1].Exists, fd.Engines[2].Exists, fd.Engines[3].Exists})
}

func TestNilPlanApplyIsSafe(t *testing.T) {
	var plan *Plan
	fd := &domain.FlightData{}
	plan.Apply(fd, nil)
	assert.True(t, plan.Empty())
	assert.Nil(t, plan.Vars())
}

// --- Registry ---

func TestBuiltinProfilesAreValid(t *testing.T) {
	r := NewRegistry()
	all := r.All()
	require.NotEmpty(t, all)

	ids := map[string]bool{}
	for _, prof := range all {
		assert.False(t, ids[prof.ID], "duplicate profile id %q", prof.ID)
		ids[prof.ID] = true
		assert.NoError(t, prof.Validate(), "profile %q", prof.ID)
		assert.Equal(t, OriginBuiltin, prof.Origin)
		assert.NotEmpty(t, prof.Mash, "profile %q mashes nothing", prof.ID)
	}

	for _, want := range []string{"fenix-a32x", "fenix-a318", "fenix-a319", "fenix-a320", "fenix-a321", "fenix-a321neo", "fslabs-a32x", "fslabs-a321neo"} {
		assert.True(t, ids[want], "expected built-in profile %q", want)
	}
}

func TestLoadDirOverridesBuiltinAndSkipsBadFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fenix-a320.json"), []byte(`{
		"id": "fenix-a320", "name": "My Fenix A320",
		"mash": {"aircraft.type": {"source": {"kind": "const", "value": "A320"}}}
	}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"id":`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`ignored`), 0o644))

	r := NewRegistry()
	require.NoError(t, r.LoadDir(dir))

	overridden, ok := r.Get("fenix-a320")
	require.True(t, ok)
	assert.Equal(t, "My Fenix A320", overridden.Name)
	assert.NotEqual(t, OriginBuiltin, overridden.Origin)

	_, stillThere := r.Get("fenix-a32x")
	assert.True(t, stillThere, "a user profile must not remove the other built-ins")
	assert.Len(t, r.LoadErrors(), 1, "the malformed file is reported, not fatal")
}

func TestLoadDirIsIdempotentAndDropsRemovedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "extra.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"id":"extra","name":"Extra","mash":{"qnh":{"source":{"kind":"const","value":1013}}}}`), 0o644))

	r := NewRegistry()
	require.NoError(t, r.LoadDir(dir))
	_, ok := r.Get("extra")
	require.True(t, ok)

	require.NoError(t, os.Remove(path))
	require.NoError(t, r.LoadDir(dir))
	_, ok = r.Get("extra")
	assert.False(t, ok, "reloading must forget profiles whose file is gone")
}

func TestLoadDirMissingDirectoryIsNotAnError(t *testing.T) {
	r := NewRegistry()
	assert.NoError(t, r.LoadDir(filepath.Join(t.TempDir(), "does-not-exist")))
	assert.NotEmpty(t, r.All())
}

func TestResolveForcedIgnoresTheSelector(t *testing.T) {
	r := NewRegistry()

	// A Cessna is nothing like a Fenix, so nothing matches on its own.
	ctx := Context{AircraftName: "Cessna 172", Simulator: SimSimConnect, EngineCount: 1}
	assert.True(t, r.Resolve(ctx, []SourceKind{SourceSimVar}).Empty())

	forced, err := r.ResolveForced("fenix-a320", ctx, []SourceKind{SourceSimVar})
	require.NoError(t, err)
	require.Len(t, forced.Bindings, 1)
	assert.Equal(t, "aircraft.type", forced.Bindings[0].Point)

	_, err = r.ResolveForced("no-such-profile", ctx, nil)
	assert.ErrorContains(t, err, "unknown aircraft profile")
}

// --- End to end over the shipped profiles ---

func TestFenixA320ResolvesFamilyAndVariant(t *testing.T) {
	r := NewRegistry()
	ctx := Context{AircraftName: "Fenix A320 IAE Lufthansa", AircraftType: "A320", Simulator: SimSimConnect, EngineCount: 2}

	plan := r.Resolve(ctx, []SourceKind{SourceSimVar})
	assert.Equal(t, []string{"fenix-a32x", "fenix-a320"}, plan.ProfileIDs())

	bindings := map[string]ResolvedBinding{}
	for _, b := range plan.Bindings {
		bindings[b.Point] = b
	}

	// An adapter that cannot read local variables falls back to the simvars.
	require.Contains(t, bindings, "autopilot.master")
	assert.Equal(t, SourceSimVar, bindings["autopilot.master"].Sources[0].Kind)
	for _, b := range plan.Bindings {
		for _, src := range b.Sources {
			assert.NotEqual(t, SourceLVar, src.Kind, "point %q should have fallen back", b.Point)
		}
	}

	require.Contains(t, bindings, "aircraft.type")
	assert.Equal(t, "A320", bindings["aircraft.type"].Sources[0].Value.Str)

	// With local variables readable, the Fenix variables win and the autopilot
	// is engaged when either flight director channel is lit.
	withLVars := r.Resolve(ctx, []SourceKind{SourceSimVar, SourceLVar})
	var master ResolvedBinding
	for _, b := range withLVars.Bindings {
		if b.Point == "autopilot.master" {
			master = b
		}
	}
	require.Len(t, master.Sources, 2, "the stock fallback does not vote alongside the aircraft's own channels")
	assert.Equal(t, ReduceOr, master.Reduce)
	assert.Equal(t, "I_FCU_AP1", master.Sources[0].Name)
	assert.Equal(t, "I_FCU_AP2", master.Sources[1].Name)

	fd := &domain.FlightData{}
	withLVars.Apply(fd, map[string]float64{"lvar:I_FCU_AP1:": 0, "lvar:I_FCU_AP2:": 1})
	assert.True(t, fd.Autopilot.Master)
}

func TestFenixVariantsReportTheirICAOType(t *testing.T) {
	r := NewRegistry()
	cases := map[string]string{
		"Fenix A318 British Airways":  "A318",
		"Fenix A319 CFM easyJet":      "A319",
		"Fenix A320 IAE Lufthansa":    "A320",
		"Fenix A321 CFM Aer Lingus":   "A321",
		"Fenix A321neo LR Aer Lingus": "A21N",
	}
	for title, want := range cases {
		t.Run(title, func(t *testing.T) {
			plan := r.Resolve(Context{AircraftName: title, Simulator: SimSimConnect, EngineCount: 2}, []SourceKind{SourceSimVar})
			fd := &domain.FlightData{}
			plan.Apply(fd, nil)
			assert.Equal(t, want, fd.AircraftType)
		})
	}
}

func TestFSLabsA321NeoOverridesTheCeoType(t *testing.T) {
	r := NewRegistry()

	ceo := r.Resolve(Context{AircraftName: "FSLabs A321 CFM Iberia", Simulator: SimSimConnect, EngineCount: 2}, []SourceKind{SourceSimVar})
	fd := &domain.FlightData{}
	ceo.Apply(fd, nil)
	assert.Equal(t, "A321", fd.AircraftType)

	neo := r.Resolve(Context{AircraftName: "FSLabs A321-NEO LEAP Wizz Air", Simulator: SimSimConnect, EngineCount: 2}, []SourceKind{SourceSimVar})
	assert.Contains(t, neo.ProfileIDs(), "fslabs-a321neo")
	fd = &domain.FlightData{}
	neo.Apply(fd, nil)
	assert.Equal(t, "A21N", fd.AircraftType)
}

func TestXPlaneProfilesDoNotMatchMSFSAircraft(t *testing.T) {
	r := NewRegistry()
	plan := r.Resolve(Context{AircraftName: "Boeing 737-800", Simulator: SimXPlane, EngineCount: 2}, []SourceKind{SourceDataRef})
	assert.Contains(t, plan.ProfileIDs(), "zibo-b738")
	assert.Contains(t, plan.ProfileIDs(), "xplane-com-833")
	for _, b := range plan.Bindings {
		for _, src := range b.Sources {
			assert.NotEqual(t, SourceSimVar, src.Kind)
		}
	}

	msfs := r.Resolve(Context{AircraftName: "PMDG 737-800 KLM", AircraftType: "B738", Simulator: SimSimConnect, EngineCount: 2}, []SourceKind{SourceSimVar})
	assert.NotContains(t, msfs.ProfileIDs(), "zibo-b738")
	assert.Contains(t, msfs.ProfileIDs(), "pmdg-737")
}

func TestContextSignatureChangesWithTheAircraft(t *testing.T) {
	a := Context{AircraftName: "Fenix A320", AircraftType: "A320", Simulator: SimSimConnect, EngineCount: 2}
	b := a
	b.AircraftName = "Fenix A321"
	assert.NotEqual(t, a.Signature(), b.Signature())
	assert.Equal(t, a.Signature(), Context{AircraftName: "Fenix A320", AircraftType: "A320", Simulator: SimSimConnect, EngineCount: 2}.Signature())
}

func TestPointsCatalogueCoversTheFlightDataStruct(t *testing.T) {
	points := Points()
	assert.NotEmpty(t, points)
	for _, id := range []string{"position.latitude", "attitude.ias", "engines.1.n1", "engines.4.running",
		"doors.5.openRatio", "radios.xpdrState", "apu.rpmPercent", "aircraft.name", "weight.fuel"} {
		_, ok := Lookup(id)
		assert.True(t, ok, "missing data point %q", id)
	}
}

// TestShippedProfilesSelectTheRightAircraft walks the built-in set with titles
// in the shape each add-on reports, and checks that the expected profiles match
// and — just as importantly — that neighbouring ones do not.
func TestShippedProfilesSelectTheRightAircraft(t *testing.T) {
	r := NewRegistry()

	cases := []struct {
		title     string
		acType    string
		simulator string
		want      []string
		reject    []string
	}{
		{title: "FlyByWire A32NX", acType: "A20N", simulator: SimSimConnect, want: []string{"fbw-a32nx"}},
		{title: "FlyByWire A380X", acType: "A388", simulator: SimSimConnect, reject: []string{"fbw-a32nx"}},
		{title: "Fenix A320 IAE Lufthansa", simulator: SimSimConnect, want: []string{"fenix-a32x", "fenix-a320"}, reject: []string{"fbw-a32nx", "fslabs-a32x"}},
		{title: "FSLabs A321-NEO LEAP", simulator: SimSimConnect, want: []string{"fslabs-a32x", "fslabs-a321neo"}, reject: []string{"fenix-a32x"}},
		{title: "PMDG 737-800 KLM", acType: "B738", simulator: SimSimConnect, want: []string{"pmdg-737"}, reject: []string{"pmdg-777", "ifly-737max", "salty-747"}},
		{title: "PMDG 777-300ER British Airways", acType: "B77W", simulator: SimSimConnect, want: []string{"pmdg-777"}, reject: []string{"pmdg-737"}},
		{title: "iFly 737 MAX 8", acType: "B38M", simulator: SimSimConnect, want: []string{"ifly-737max"}, reject: []string{"pmdg-737"}},
		{title: "iniBuilds A350-900 Qatar", simulator: SimSimConnect, want: []string{"inibuilds-a350"}, reject: []string{"inibuilds-a300", "inibuilds-a310"}},
		{title: "iniBuilds A310-300 Air Transat", simulator: SimSimConnect, want: []string{"inibuilds-a310"}, reject: []string{"inibuilds-a350"}},
		{title: "TFDi Design MD-11 Lufthansa Cargo", simulator: SimSimConnect, want: []string{"tfdi-md11"}, reject: []string{"leonardo-md82"}},
		{title: "Leonardo MaddogX MD-82 Alitalia", simulator: SimSimConnect, want: []string{"leonardo-md82"}, reject: []string{"tfdi-md11"}},
		{title: "Aerosoft CRJ 900 Lufthansa Regional", simulator: SimSimConnect, want: []string{"aerosoft-crj"}},
		{title: "Salty 747-8i Lufthansa", simulator: SimSimConnect, want: []string{"salty-747"}},
		{title: "Just Flight BAe 146-300 Flybe", simulator: SimSimConnect, want: []string{"justflight-bae146"}},
		{title: "ATR 72-600 Aer Lingus Regional", simulator: SimSimConnect, want: []string{"msfs-atr72"}},
		{title: "Boeing 737-800X Zibo mod", acType: "B738", simulator: SimXPlane, want: []string{"zibo-b738", "xplane-com-833"}, reject: []string{"pmdg-737"}},
	}

	supported := map[string][]SourceKind{
		SimSimConnect: {SourceSimVar, SourceLVar},
		SimXPlane:     {SourceDataRef},
	}

	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			plan := r.Resolve(Context{
				AircraftName: tc.title,
				AircraftType: tc.acType,
				Simulator:    tc.simulator,
				EngineCount:  2,
			}, supported[tc.simulator])

			for _, want := range tc.want {
				assert.Contains(t, plan.ProfileIDs(), want)
			}
			for _, reject := range tc.reject {
				assert.NotContains(t, plan.ProfileIDs(), reject)
			}
		})
	}
}

// TestShippedProfilesDegradeToTheStockVariables is the safety net for the
// aircraft profiles that read local variables: an adapter that can only read
// simulation variables — an older simulator build, or X-Plane — must still end
// up with every data point bound to something it can actually read.
func TestShippedProfilesDegradeToTheStockVariables(t *testing.T) {
	r := NewRegistry()

	for _, prof := range r.All() {
		t.Run(prof.ID, func(t *testing.T) {
			for _, sim := range []struct {
				name      string
				supported []SourceKind
			}{
				{SimSimConnect, []SourceKind{SourceSimVar}},
				{SimXPlane, []SourceKind{SourceDataRef}},
			} {
				forced, err := r.ResolveForced(prof.ID, Context{Simulator: sim.name}, sim.supported)
				require.NoError(t, err)
				for _, b := range forced.Bindings {
					for _, src := range b.Sources {
						assert.Contains(t, append(sim.supported, SourceConst), src.Kind,
							"%s on %s: point %q resolved to an unreadable source", prof.ID, sim.name, b.Point)
					}
				}
			}
		})
	}
}

// TestPMDG737FallsBackAndCombines checks the shape of a profile that lists both
// generations of PMDG variable naming: with local variables readable every
// candidate feeds the reduce, and without them only the stock simvar is left.
func TestPMDG737FallsBackAndCombines(t *testing.T) {
	r := NewRegistry()
	ctx := Context{AircraftName: "PMDG 737-800 KLM", AircraftType: "B738", Simulator: SimSimConnect, EngineCount: 2}

	find := func(plan *Plan, point string) (ResolvedBinding, bool) {
		for _, b := range plan.Bindings {
			if b.Point == point {
				return b, true
			}
		}
		return ResolvedBinding{}, false
	}

	simOnly := r.Resolve(ctx, []SourceKind{SourceSimVar})
	master, ok := find(simOnly, "autopilot.master")
	require.True(t, ok)
	require.Len(t, master.Sources, 1)
	assert.Equal(t, "AUTOPILOT MASTER", master.Sources[0].Name)

	withLVars := r.Resolve(ctx, []SourceKind{SourceSimVar, SourceLVar})
	master, ok = find(withLVars, "autopilot.master")
	require.True(t, ok)
	assert.Equal(t, ReduceOr, master.Reduce)
	assert.Len(t, master.Sources, 4, "both PMDG generations, and only those")
	for _, src := range master.Sources {
		assert.Equal(t, SourceLVar, src.Kind,
			"a reduce combines one tier; the stock variable is the fallback, not a vote")
	}

	// Either MCP channel engages the autopilot, and a blank V/S window reads zero.
	fd := &domain.FlightData{}
	withLVars.Apply(fd, map[string]float64{
		"lvar:ngx_MCP_CMDA:":     0,
		"lvar:ngx_MCP_CMDB:":     1,
		"lvar:ngx_MCP_VSwindow:": -20000,
	})
	assert.True(t, fd.Autopilot.Master)
	assert.Zero(t, fd.Autopilot.VS)
}

// TestGeneratedProfilesOnlyBindAircraftVariables guards the invariant that
// makes the weekly refresh safe to merge on sight: a generated draft may only
// bind something the adapter does not already read. Binding a stock variable
// would either do nothing or, worse, quietly replace a correct reading with a
// mis-mapped one.
func TestGeneratedProfilesOnlyBindAircraftVariables(t *testing.T) {
	for _, prof := range NewRegistry().All() {
		if !prof.Generated {
			continue
		}
		t.Run(prof.ID, func(t *testing.T) {
			assert.Less(t, prof.Priority, 100,
				"a draft must never outrank a profile someone confirmed in the simulator")

			for point, entry := range prof.Mash {
				require.NotEmpty(t, entry.Bindings)
				primary := entry.Bindings[0]
				switch primary.Source.Kind {
				case SourceLVar:
					// An add-on's own variable: exactly what a draft is for.
				case SourceDataRef:
					assert.False(t, strings.HasPrefix(primary.Source.Name, "sim/"),
						"%s: %q reads the stock dataref %q, which the adapter already collects",
						prof.ID, point, primary.Source.Name)
				default:
					t.Errorf("%s: %q is fed by %q, which is not an aircraft variable",
						prof.ID, point, primary.Source.Kind)
				}
			}

			// The fallback is what makes an unverified draft harmless.
			for point, entry := range prof.Mash {
				last := entry.Bindings[len(entry.Bindings)-1]
				assert.NotEqual(t, SourceLVar, last.Source.Kind,
					"%s: %q has no stock fallback behind its aircraft variables", prof.ID, point)
			}
		})
	}
}

// TestReduceCombinesOneTierOnly documents why a fallback must not vote: the
// aircraft says the autopilot is off, and a stale stock variable saying it is
// on must not be able to override that.
func TestReduceCombinesOneTierOnly(t *testing.T) {
	prof := mustParse(t, `{
		"id": "tiers", "name": "Tiers",
		"mash": {
			"autopilot.master": {
				"reduce": "or",
				"bindings": [
					{"source": {"kind": "lvar", "name": "AP1"}, "transform": [{"op": "bool"}]},
					{"source": {"kind": "lvar", "name": "AP2"}, "transform": [{"op": "bool"}]},
					{"source": {"kind": "simvar", "name": "AUTOPILOT MASTER", "unit": "Bool"}, "transform": [{"op": "bool"}]}
				]
			}
		}
	}`)

	withLVars := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar, SourceLVar})
	require.Len(t, withLVars.Bindings[0].Sources, 2)

	fd := &domain.FlightData{}
	withLVars.Apply(fd, map[string]float64{
		"lvar:AP1:": 0, "lvar:AP2:": 0,
		"simvar:AUTOPILOT MASTER:Bool": 1,
	})
	assert.False(t, fd.Autopilot.Master, "the aircraft's own channels decide, not the fallback")

	// With local variables unreadable the stock variable becomes the only tier.
	simOnly := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceSimVar})
	require.Len(t, simOnly.Bindings[0].Sources, 1)
	fd = &domain.FlightData{}
	simOnly.Apply(fd, map[string]float64{"simvar:AUTOPILOT MASTER:Bool": 1})
	assert.True(t, fd.Autopilot.Master)
}

// TestReduceAndNeedsEveryReading covers the failure mode SimConnect's habit of
// creating a missing local variable would otherwise produce: two of three gear
// greens reporting must not be enough to call the gear down.
func TestReduceAndNeedsEveryReading(t *testing.T) {
	prof := mustParse(t, `{
		"id": "greens", "name": "Greens",
		"mash": {
			"controls.gearDown": {
				"reduce": "and",
				"bindings": [
					{"source": {"kind": "lvar", "name": "GEAR_N"}, "transform": [{"op": "gt", "value": 0}]},
					{"source": {"kind": "lvar", "name": "GEAR_L"}, "transform": [{"op": "gt", "value": 0}]},
					{"source": {"kind": "lvar", "name": "GEAR_R"}, "transform": [{"op": "gt", "value": 0}]}
				]
			}
		}
	}`)
	plan := Resolve([]*Profile{prof}, Context{Simulator: SimSimConnect}, []SourceKind{SourceLVar})

	fd := &domain.FlightData{}
	fd.Controls.GearDown = true // what the adapter read for itself
	plan.Apply(fd, map[string]float64{"lvar:GEAR_N:": 1, "lvar:GEAR_L:": 1})
	assert.True(t, fd.Controls.GearDown, "an absent third green is unknown, not false")

	plan.Apply(fd, map[string]float64{"lvar:GEAR_N:": 1, "lvar:GEAR_L:": 1, "lvar:GEAR_R:": 0})
	assert.False(t, fd.Controls.GearDown, "once every green reports, the answer is theirs")
}
