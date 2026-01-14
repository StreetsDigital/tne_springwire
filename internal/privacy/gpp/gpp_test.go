package gpp

import (
	"testing"
)

func TestParseGPPString_Empty(t *testing.T) {
	_, err := Parse("")
	if err != ErrEmptyGPPString {
		t.Errorf("expected ErrEmptyGPPString, got %v", err)
	}
}

func TestParseGPPString_InvalidHeader(t *testing.T) {
	// Invalid base64
	_, err := Parse("!!!invalid!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestBitReader(t *testing.T) {
	// Test bit reader with known values
	data := []byte{0b10110100, 0b11001010}
	reader := newBitReader(data)

	// First byte: 10110100
	if !reader.readBool() { // 1
		t.Error("expected true for bit 0")
	}
	if reader.readBool() { // 0
		t.Error("expected false for bit 1")
	}

	// Read 6 bits: 110100 = 52
	val := reader.readInt(6)
	if val != 52 {
		t.Errorf("expected 52, got %d", val)
	}
}

func TestUSNationalSection_OptOutChecks(t *testing.T) {
	section := &USNationalSection{
		Version:                   1,
		SaleOptOut:                OptOutYes,
		SharingOptOut:             OptOutNo,
		TargetedAdvertisingOptOut: OptOutYes,
		MspaCoveredTransaction:    OptOutYes,
		Gpc:                       true,
	}

	if !section.HasSaleOptOut() {
		t.Error("expected HasSaleOptOut to be true")
	}
	if section.HasSharingOptOut() {
		t.Error("expected HasSharingOptOut to be false")
	}
	if !section.HasTargetedAdOptOut() {
		t.Error("expected HasTargetedAdOptOut to be true")
	}
	if !section.IsCoveredTransaction() {
		t.Error("expected IsCoveredTransaction to be true")
	}
	if !section.HasGPC() {
		t.Error("expected HasGPC to be true")
	}
}

func TestUSStateSection_OptOutChecks(t *testing.T) {
	section := &USStateSection{
		SectionID:                 SectionUSCA,
		Version:                   1,
		SaleOptOut:                OptOutNo,
		TargetedAdvertisingOptOut: OptOutYes,
		Gpc:                       false,
	}

	if section.HasSaleOptOut() {
		t.Error("expected HasSaleOptOut to be false")
	}
	if !section.HasTargetedAdOptOut() {
		t.Error("expected HasTargetedAdOptOut to be true")
	}
}

func TestEnforceForActivity_NilGPP(t *testing.T) {
	result := EnforceForActivity(nil, []int{SectionUSNat}, ActivityBidRequest)
	if !result.Allowed {
		t.Error("expected Allowed for nil GPP")
	}
}

func TestEnforceForActivity_SaleOptOut(t *testing.T) {
	gpp := &ParsedGPP{
		Version:    1,
		SectionIDs: []int{SectionUSNat},
		Sections: map[int]Section{
			SectionUSNat: &USNationalSection{
				Version:                1,
				SaleOptOut:             OptOutYes,
				MspaCoveredTransaction: OptOutYes,
			},
		},
	}

	result := EnforceForActivity(gpp, []int{SectionUSNat}, ActivitySellData)
	if result.Allowed {
		t.Error("expected not Allowed when sale opt-out is set")
	}
	if !result.SaleBlocked {
		t.Error("expected SaleBlocked to be true")
	}
}

func TestEnforceForActivity_TargetedAdOptOut(t *testing.T) {
	gpp := &ParsedGPP{
		Version:    1,
		SectionIDs: []int{SectionUSNat},
		Sections: map[int]Section{
			SectionUSNat: &USNationalSection{
				Version:                   1,
				TargetedAdvertisingOptOut: OptOutYes,
				MspaCoveredTransaction:    OptOutYes,
			},
		},
	}

	result := EnforceForActivity(gpp, []int{SectionUSNat}, ActivityTargetedAdvertise)
	if result.Allowed {
		t.Error("expected not Allowed when targeted ad opt-out is set")
	}
	if !result.TargetedAdsBlocked {
		t.Error("expected TargetedAdsBlocked to be true")
	}
}

func TestEnforceForActivity_NotCoveredTransaction(t *testing.T) {
	gpp := &ParsedGPP{
		Version:    1,
		SectionIDs: []int{SectionUSNat},
		Sections: map[int]Section{
			SectionUSNat: &USNationalSection{
				Version:                1,
				SaleOptOut:             OptOutYes,
				MspaCoveredTransaction: OptOutNo, // Not covered
			},
		},
	}

	result := EnforceForActivity(gpp, []int{SectionUSNat}, ActivitySellData)
	if !result.Allowed {
		t.Error("expected Allowed when not a covered transaction")
	}
}

func TestEnforceForActivity_GPC(t *testing.T) {
	gpp := &ParsedGPP{
		Version:    1,
		SectionIDs: []int{SectionUSNat},
		Sections: map[int]Section{
			SectionUSNat: &USNationalSection{
				Version:                2,
				Gpc:                    true,
				MspaCoveredTransaction: OptOutYes,
			},
		},
	}

	result := EnforceForActivity(gpp, []int{SectionUSNat}, ActivityUserSync)
	if result.Allowed {
		t.Error("expected not Allowed when GPC is set")
	}
}

func TestShouldBlockBidder_Empty(t *testing.T) {
	blocked, _ := ShouldBlockBidder("", nil)
	if blocked {
		t.Error("expected not blocked for empty GPP string")
	}
}

func TestGetUSStateForSectionID(t *testing.T) {
	tests := []struct {
		sectionID int
		expected  string
	}{
		{SectionUSNat, "US"},
		{SectionUSCA, "CA"},
		{SectionUSVA, "VA"},
		{SectionUSCO, "CO"},
		{SectionUSUT, "UT"},
		{SectionUSCT, "CT"},
		{0, ""},
		{999, ""},
	}

	for _, tc := range tests {
		result := GetUSStateForSectionID(tc.sectionID)
		if result != tc.expected {
			t.Errorf("GetUSStateForSectionID(%d) = %s, expected %s", tc.sectionID, result, tc.expected)
		}
	}
}

func TestGetSectionIDForUSState(t *testing.T) {
	tests := []struct {
		state    string
		expected int
	}{
		{"CA", SectionUSCA},
		{"VA", SectionUSVA},
		{"CO", SectionUSCO},
		{"UT", SectionUSUT},
		{"CT", SectionUSCT},
		{"XX", 0},
		{"", 0},
	}

	for _, tc := range tests {
		result := GetSectionIDForUSState(tc.state)
		if result != tc.expected {
			t.Errorf("GetSectionIDForUSState(%s) = %d, expected %d", tc.state, result, tc.expected)
		}
	}
}

func TestIsUSPrivacySection(t *testing.T) {
	tests := []struct {
		sectionID int
		expected  bool
	}{
		{SectionUSNat, true},
		{SectionUSCA, true},
		{SectionUSRI, true},
		{SectionTCFEUv2, false},
		{0, false},
		{100, false},
	}

	for _, tc := range tests {
		result := IsUSPrivacySection(tc.sectionID)
		if result != tc.expected {
			t.Errorf("IsUSPrivacySection(%d) = %v, expected %v", tc.sectionID, result, tc.expected)
		}
	}
}

func TestContainsApplicableSID(t *testing.T) {
	sids := []int{SectionUSNat, SectionUSCA, SectionUSVA}

	if !ContainsApplicableSID(sids, SectionUSNat) {
		t.Error("expected to find SectionUSNat")
	}
	if !ContainsApplicableSID(sids, SectionUSCA) {
		t.Error("expected to find SectionUSCA")
	}
	if ContainsApplicableSID(sids, SectionUSCO) {
		t.Error("expected not to find SectionUSCO")
	}
	if ContainsApplicableSID(nil, SectionUSNat) {
		t.Error("expected not to find in nil slice")
	}
}

func TestOptOutValue_String(t *testing.T) {
	// Test that opt-out values are correct
	if OptOutNotApplicable != 0 {
		t.Error("OptOutNotApplicable should be 0")
	}
	if OptOutYes != 1 {
		t.Error("OptOutYes should be 1")
	}
	if OptOutNo != 2 {
		t.Error("OptOutNo should be 2")
	}
}

func TestSectionIDs(t *testing.T) {
	// Verify section IDs match IAB specification
	tests := []struct {
		name     string
		id       int
		expected int
	}{
		{"TCF EU v2", SectionTCFEUv2, 2},
		{"TCF CA v1", SectionTCFCAv1, 5},
		{"US National", SectionUSNat, 7},
		{"US California", SectionUSCA, 8},
		{"US Virginia", SectionUSVA, 9},
		{"US Colorado", SectionUSCO, 10},
		{"US Utah", SectionUSUT, 11},
		{"US Connecticut", SectionUSCT, 12},
	}

	for _, tc := range tests {
		if tc.id != tc.expected {
			t.Errorf("%s section ID = %d, expected %d", tc.name, tc.id, tc.expected)
		}
	}
}

func TestUSNationalSection_GetID(t *testing.T) {
	section := &USNationalSection{Version: 1}
	if section.GetID() != SectionUSNat {
		t.Errorf("GetID() = %d, expected %d", section.GetID(), SectionUSNat)
	}
}

func TestUSNationalSection_GetVersion(t *testing.T) {
	section := &USNationalSection{Version: 2}
	if section.GetVersion() != 2 {
		t.Errorf("GetVersion() = %d, expected 2", section.GetVersion())
	}
}

func TestUSStateSection_GetID(t *testing.T) {
	section := &USStateSection{SectionID: SectionUSCA, Version: 1}
	if section.GetID() != SectionUSCA {
		t.Errorf("GetID() = %d, expected %d", section.GetID(), SectionUSCA)
	}
}

func TestUSStateSection_GetVersion(t *testing.T) {
	section := &USStateSection{SectionID: SectionUSCA, Version: 1}
	if section.GetVersion() != 1 {
		t.Errorf("GetVersion() = %d, expected 1", section.GetVersion())
	}
}

func TestEnforceUSState_SaleOptOut(t *testing.T) {
	section := &USStateSection{
		SectionID:              SectionUSCA,
		Version:                1,
		SaleOptOut:             OptOutYes,
		MspaCoveredTransaction: OptOutYes,
	}

	result := &EnforcementResult{Allowed: true}
	enforceUSState(section, ActivitySellData, result)

	if result.Allowed {
		t.Error("expected not Allowed when sale opt-out is set")
	}
	if !result.SaleBlocked {
		t.Error("expected SaleBlocked to be true")
	}
}

func TestEnforceUSState_SharingOptOut(t *testing.T) {
	section := &USStateSection{
		SectionID:              SectionUSCA,
		Version:                1,
		SharingOptOut:          OptOutYes,
		MspaCoveredTransaction: OptOutYes,
	}

	result := &EnforcementResult{Allowed: true}
	enforceUSState(section, ActivityShareData, result)

	if result.Allowed {
		t.Error("expected not Allowed when sharing opt-out is set")
	}
	if !result.SharingBlocked {
		t.Error("expected SharingBlocked to be true")
	}
}

func TestEnforceUSState_GPC(t *testing.T) {
	section := &USStateSection{
		SectionID:              SectionUSCA,
		Version:                1,
		Gpc:                    true,
		MspaCoveredTransaction: OptOutYes,
	}

	result := &EnforcementResult{Allowed: true}
	enforceUSState(section, ActivityUserSync, result)

	if result.Allowed {
		t.Error("expected not Allowed when GPC is set")
	}
}

func TestSensitiveCategories(t *testing.T) {
	// Test California has 12 categories
	if getSensitiveCategoriesForState(SectionUSCA) != 12 {
		t.Error("California should have 12 sensitive data categories")
	}

	// Test other states have 8 categories
	if getSensitiveCategoriesForState(SectionUSVA) != 8 {
		t.Error("Virginia should have 8 sensitive data categories")
	}
}

func TestChildCategories(t *testing.T) {
	// Test California has 2 child consent categories
	if getChildCategoriesForState(SectionUSCA) != 2 {
		t.Error("California should have 2 child consent categories")
	}
}
