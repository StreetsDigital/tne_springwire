package gpp

// EnforcementResult represents the result of GPP enforcement
type EnforcementResult struct {
	// Allowed indicates if the activity is allowed
	Allowed bool
	// Reason provides explanation if not allowed
	Reason string
	// ApplicableSections lists which sections were evaluated
	ApplicableSections []int
	// SaleBlocked indicates if sale of data is blocked
	SaleBlocked bool
	// TargetedAdsBlocked indicates if targeted advertising is blocked
	TargetedAdsBlocked bool
	// SharingBlocked indicates if data sharing is blocked
	SharingBlocked bool
}

// EnforceForActivity evaluates GPP consent for a specific activity
func EnforceForActivity(gpp *ParsedGPP, applicableSIDs []int, activity Activity) *EnforcementResult {
	result := &EnforcementResult{
		Allowed:            true,
		ApplicableSections: applicableSIDs,
	}

	if gpp == nil {
		return result
	}

	// Check each applicable section
	for _, sid := range applicableSIDs {
		section, exists := gpp.Sections[sid]
		if !exists {
			continue
		}

		switch s := section.(type) {
		case *USNationalSection:
			enforceUSNational(s, activity, result)
		case *USStateSection:
			enforceUSState(s, activity, result)
		}

		// If any section blocks, we're blocked
		if !result.Allowed {
			break
		}
	}

	return result
}

// Activity represents different types of advertising activities
type Activity string

const (
	ActivityTransmitUserData   Activity = "transmitUserData"
	ActivitySellData           Activity = "sellData"
	ActivityShareData          Activity = "shareData"
	ActivityTargetedAdvertise  Activity = "targetedAdvertising"
	ActivityProcessSensitive   Activity = "processSensitiveData"
	ActivityProcessChildData   Activity = "processChildData"
	ActivityBidRequest         Activity = "bidRequest"
	ActivityUserSync           Activity = "userSync"
	ActivityEnrichWithEIDs     Activity = "enrichWithEIDs"
	ActivityReportAnalytics    Activity = "reportAnalytics"
)

// enforceUSNational applies US National section rules
func enforceUSNational(section *USNationalSection, activity Activity, result *EnforcementResult) {
	// Check if this is a covered transaction
	if section.MspaCoveredTransaction != OptOutYes {
		// Not a covered transaction - no restrictions
		return
	}

	switch activity {
	case ActivitySellData, ActivityBidRequest:
		if section.HasSaleOptOut() {
			result.Allowed = false
			result.SaleBlocked = true
			result.Reason = "US National: User has opted out of sale of personal data"
		}

	case ActivityShareData:
		if section.HasSharingOptOut() {
			result.Allowed = false
			result.SharingBlocked = true
			result.Reason = "US National: User has opted out of sharing personal data"
		}

	case ActivityTargetedAdvertise:
		if section.HasTargetedAdOptOut() {
			result.Allowed = false
			result.TargetedAdsBlocked = true
			result.Reason = "US National: User has opted out of targeted advertising"
		}

	case ActivityTransmitUserData, ActivityUserSync, ActivityEnrichWithEIDs:
		// Check GPC signal
		if section.HasGPC() {
			result.Allowed = false
			result.Reason = "US National: Global Privacy Control signal is set"
		}
		// Also check sale opt-out for transmission
		if section.HasSaleOptOut() {
			result.Allowed = false
			result.SaleBlocked = true
			result.Reason = "US National: User has opted out of sale of personal data"
		}

	case ActivityProcessSensitive:
		// Check if any sensitive data category is opted out
		for i, consent := range section.SensitiveDataProcessing {
			if consent == OptOutYes {
				result.Allowed = false
				result.Reason = "US National: User has opted out of sensitive data processing (category " + string(rune('0'+i)) + ")"
				break
			}
		}

	case ActivityProcessChildData:
		// Check child consent
		for _, consent := range section.KnownChildSensitiveDataConsents {
			if consent == OptOutYes || consent == OptOutNotApplicable {
				result.Allowed = false
				result.Reason = "US National: Child data processing not consented"
				break
			}
		}
	}
}

// enforceUSState applies state-specific section rules
func enforceUSState(section *USStateSection, activity Activity, result *EnforcementResult) {
	// Check if covered transaction
	if section.MspaCoveredTransaction != OptOutYes {
		return
	}

	switch activity {
	case ActivitySellData, ActivityBidRequest:
		if section.HasSaleOptOut() {
			result.Allowed = false
			result.SaleBlocked = true
			result.Reason = "US State: User has opted out of sale of personal data"
		}

	case ActivityShareData:
		// Only some states have sharing opt-out
		if section.SharingOptOut == OptOutYes {
			result.Allowed = false
			result.SharingBlocked = true
			result.Reason = "US State: User has opted out of sharing personal data"
		}

	case ActivityTargetedAdvertise:
		if section.HasTargetedAdOptOut() {
			result.Allowed = false
			result.TargetedAdsBlocked = true
			result.Reason = "US State: User has opted out of targeted advertising"
		}

	case ActivityTransmitUserData, ActivityUserSync, ActivityEnrichWithEIDs:
		if section.Gpc {
			result.Allowed = false
			result.Reason = "US State: Global Privacy Control signal is set"
		}
		if section.HasSaleOptOut() {
			result.Allowed = false
			result.SaleBlocked = true
			result.Reason = "US State: User has opted out of sale of personal data"
		}
	}
}

// ShouldBlockBidder evaluates if a bidder should be blocked based on GPP
func ShouldBlockBidder(gppString string, applicableSIDs []int) (bool, string) {
	if gppString == "" {
		return false, ""
	}

	gpp, err := Parse(gppString)
	if err != nil {
		// Invalid GPP string - don't block, let it pass
		return false, ""
	}

	result := EnforceForActivity(gpp, applicableSIDs, ActivityBidRequest)
	return !result.Allowed, result.Reason
}

// GetUSStateForSectionID returns the US state abbreviation for a section ID
func GetUSStateForSectionID(sectionID int) string {
	switch sectionID {
	case SectionUSNat:
		return "US" // National
	case SectionUSCA:
		return "CA"
	case SectionUSVA:
		return "VA"
	case SectionUSCO:
		return "CO"
	case SectionUSUT:
		return "UT"
	case SectionUSCT:
		return "CT"
	case SectionUSFL:
		return "FL"
	case SectionUSMT:
		return "MT"
	case SectionUSOr:
		return "OR"
	case SectionUSTX:
		return "TX"
	case SectionUSDE:
		return "DE"
	case SectionUSIA:
		return "IA"
	case SectionUSNE:
		return "NE"
	case SectionUSNH:
		return "NH"
	case SectionUSNJ:
		return "NJ"
	case SectionUSTN:
		return "TN"
	case SectionUSMN:
		return "MN"
	case SectionUSMD:
		return "MD"
	case SectionUSIN:
		return "IN"
	case SectionUSKY:
		return "KY"
	case SectionUSRI:
		return "RI"
	default:
		return ""
	}
}

// GetSectionIDForUSState returns the section ID for a US state
func GetSectionIDForUSState(state string) int {
	switch state {
	case "CA":
		return SectionUSCA
	case "VA":
		return SectionUSVA
	case "CO":
		return SectionUSCO
	case "UT":
		return SectionUSUT
	case "CT":
		return SectionUSCT
	case "FL":
		return SectionUSFL
	case "MT":
		return SectionUSMT
	case "OR":
		return SectionUSOr
	case "TX":
		return SectionUSTX
	case "DE":
		return SectionUSDE
	case "IA":
		return SectionUSIA
	case "NE":
		return SectionUSNE
	case "NH":
		return SectionUSNH
	case "NJ":
		return SectionUSNJ
	case "TN":
		return SectionUSTN
	case "MN":
		return SectionUSMN
	case "MD":
		return SectionUSMD
	case "IN":
		return SectionUSIN
	case "KY":
		return SectionUSKY
	case "RI":
		return SectionUSRI
	default:
		return 0
	}
}

// IsUSPrivacySection returns true if the section ID is a US privacy section
func IsUSPrivacySection(sectionID int) bool {
	return sectionID >= SectionUSNat && sectionID <= SectionUSRI
}

// ContainsApplicableSID checks if any of the applicable SIDs match the given section
func ContainsApplicableSID(applicableSIDs []int, sectionID int) bool {
	for _, sid := range applicableSIDs {
		if sid == sectionID {
			return true
		}
	}
	return false
}
