// Package gpp provides parsing and enforcement for IAB Global Privacy Platform (GPP) strings
// GPP is the IAB's unified privacy framework that consolidates GDPR, CCPA, and other regional regulations.
// Specification: https://github.com/InteractiveAdvertisingBureau/Global-Privacy-Platform
package gpp

import (
	"encoding/base64"
	"errors"
	"strings"
)

// Section IDs as defined by IAB GPP specification
const (
	SectionTCFEUv2     = 2  // EU TCF v2 (GDPR)
	SectionTCFCAv1     = 5  // Canada TCF
	SectionUSNat       = 7  // US National (MSPA)
	SectionUSCA        = 8  // California (CPRA)
	SectionUSVA        = 9  // Virginia (VCDPA)
	SectionUSCO        = 10 // Colorado (CPA)
	SectionUSUT        = 11 // Utah (UCPA)
	SectionUSCT        = 12 // Connecticut (CTDPA)
	SectionUSFL        = 13 // Florida
	SectionUSMT        = 14 // Montana
	SectionUSOr        = 15 // Oregon
	SectionUSTX        = 16 // Texas
	SectionUSDE        = 17 // Delaware
	SectionUSIA        = 18 // Iowa
	SectionUSNE        = 19 // Nebraska
	SectionUSNH        = 20 // New Hampshire
	SectionUSNJ        = 21 // New Jersey
	SectionUSTN        = 22 // Tennessee
	SectionUSMN        = 23 // Minnesota
	SectionUSMD        = 24 // Maryland
	SectionUSIN        = 25 // Indiana
	SectionUSKY        = 26 // Kentucky
	SectionUSRI        = 27 // Rhode Island
)

// OptOutValue represents the possible values for opt-out fields
type OptOutValue int

const (
	OptOutNotApplicable OptOutValue = 0 // N/A - not applicable
	OptOutYes           OptOutValue = 1 // User has opted out
	OptOutNo            OptOutValue = 2 // User has not opted out
)

// ParsedGPP represents a fully parsed GPP string
type ParsedGPP struct {
	// Version of the GPP header
	Version int
	// SectionIDs contains all section IDs present in the GPP string
	SectionIDs []int
	// Sections maps section ID to parsed section data
	Sections map[int]Section
	// RawString is the original GPP string
	RawString string
}

// Section is an interface implemented by all GPP section types
type Section interface {
	GetID() int
	GetVersion() int
}

// USNationalSection represents Section 7 (US National Privacy - MSPA)
type USNationalSection struct {
	Version                             int
	SharingNotice                       OptOutValue
	SaleOptOutNotice                    OptOutValue
	SharingOptOutNotice                 OptOutValue
	TargetedAdvertisingOptOutNotice     OptOutValue
	SensitiveDataProcessingOptOutNotice OptOutValue
	SensitiveDataLimitUseNotice         OptOutValue
	SaleOptOut                          OptOutValue
	SharingOptOut                       OptOutValue
	TargetedAdvertisingOptOut           OptOutValue
	SensitiveDataProcessing             []OptOutValue // 16 categories
	KnownChildSensitiveDataConsents     []OptOutValue // 3 age groups (v1) or 2 (v2)
	PersonalDataConsents                OptOutValue
	MspaCoveredTransaction              OptOutValue
	MspaOptOutOptionMode                OptOutValue
	MspaServiceProviderMode             OptOutValue
	Gpc                                 bool // Global Privacy Control signal (v2+)
}

func (s *USNationalSection) GetID() int      { return SectionUSNat }
func (s *USNationalSection) GetVersion() int { return s.Version }

// HasSaleOptOut returns true if user has opted out of sale of personal data
func (s *USNationalSection) HasSaleOptOut() bool {
	return s.SaleOptOut == OptOutYes
}

// HasSharingOptOut returns true if user has opted out of sharing personal data
func (s *USNationalSection) HasSharingOptOut() bool {
	return s.SharingOptOut == OptOutYes
}

// HasTargetedAdOptOut returns true if user has opted out of targeted advertising
func (s *USNationalSection) HasTargetedAdOptOut() bool {
	return s.TargetedAdvertisingOptOut == OptOutYes
}

// IsCoveredTransaction returns true if this is an MSPA covered transaction
func (s *USNationalSection) IsCoveredTransaction() bool {
	return s.MspaCoveredTransaction == OptOutYes
}

// HasGPC returns true if Global Privacy Control signal is set
func (s *USNationalSection) HasGPC() bool {
	return s.Gpc
}

// USStateSection represents state-specific sections (8-27)
type USStateSection struct {
	SectionID                           int
	Version                             int
	SaleOptOutNotice                    OptOutValue
	SharingOptOutNotice                 OptOutValue
	TargetedAdvertisingOptOutNotice     OptOutValue
	SensitiveDataProcessingOptOutNotice OptOutValue
	SaleOptOut                          OptOutValue
	SharingOptOut                       OptOutValue // Not in all states
	TargetedAdvertisingOptOut           OptOutValue
	SensitiveDataProcessing             []OptOutValue
	KnownChildSensitiveDataConsents     []OptOutValue
	MspaCoveredTransaction              OptOutValue
	MspaOptOutOptionMode                OptOutValue
	MspaServiceProviderMode             OptOutValue
	Gpc                                 bool
}

func (s *USStateSection) GetID() int      { return s.SectionID }
func (s *USStateSection) GetVersion() int { return s.Version }

// HasSaleOptOut returns true if user has opted out of sale
func (s *USStateSection) HasSaleOptOut() bool {
	return s.SaleOptOut == OptOutYes
}

// HasTargetedAdOptOut returns true if user has opted out of targeted advertising
func (s *USStateSection) HasTargetedAdOptOut() bool {
	return s.TargetedAdvertisingOptOut == OptOutYes
}

// Errors
var (
	ErrEmptyGPPString     = errors.New("GPP string is empty")
	ErrInvalidGPPHeader   = errors.New("invalid GPP header")
	ErrInvalidGPPEncoding = errors.New("invalid GPP encoding")
	ErrInvalidSection     = errors.New("invalid GPP section")
	ErrUnsupportedVersion = errors.New("unsupported GPP version")
)

// Parse parses a complete GPP string and returns all sections
func Parse(gppString string) (*ParsedGPP, error) {
	if gppString == "" {
		return nil, ErrEmptyGPPString
	}

	// Split by tilde delimiter
	parts := strings.Split(gppString, "~")
	if len(parts) < 1 {
		return nil, ErrInvalidGPPHeader
	}

	// Parse header (first segment)
	header, sectionIDs, err := parseHeader(parts[0])
	if err != nil {
		return nil, err
	}

	result := &ParsedGPP{
		Version:    header.Version,
		SectionIDs: sectionIDs,
		Sections:   make(map[int]Section),
		RawString:  gppString,
	}

	// Parse each section (segments after header)
	for i, sectionID := range sectionIDs {
		if i+1 >= len(parts) {
			// Section ID declared in header but no data
			continue
		}

		section, err := parseSection(sectionID, parts[i+1])
		if err != nil {
			// Log error but continue parsing other sections
			continue
		}
		if section != nil {
			result.Sections[sectionID] = section
		}
	}

	return result, nil
}

// gppHeader represents the parsed GPP header
type gppHeader struct {
	Type    int // Should be 3 for GPP
	Version int // GPP version (currently 1)
}

// parseHeader parses the GPP header segment
func parseHeader(headerStr string) (*gppHeader, []int, error) {
	if headerStr == "" {
		return nil, nil, ErrInvalidGPPHeader
	}

	// Decode base64
	decoded, err := base64.RawURLEncoding.DecodeString(headerStr)
	if err != nil {
		// Try standard base64
		decoded, err = base64.StdEncoding.DecodeString(headerStr)
		if err != nil {
			return nil, nil, ErrInvalidGPPEncoding
		}
	}

	if len(decoded) < 2 {
		return nil, nil, ErrInvalidGPPHeader
	}

	reader := newBitReader(decoded)

	// Type (6 bits) - should be 3 for GPP
	headerType := reader.readInt(6)
	if headerType != 3 {
		return nil, nil, ErrInvalidGPPHeader
	}

	// Version (6 bits)
	version := reader.readInt(6)

	// Section IDs - Fibonacci encoded range
	sectionIDs, err := parseFibonacciIntRange(reader)
	if err != nil {
		return nil, nil, err
	}

	return &gppHeader{
		Type:    headerType,
		Version: version,
	}, sectionIDs, nil
}

// parseSection parses a single GPP section by ID
func parseSection(sectionID int, sectionData string) (Section, error) {
	if sectionData == "" {
		return nil, ErrInvalidSection
	}

	switch sectionID {
	case SectionUSNat:
		return parseUSNationalSection(sectionData)
	case SectionUSCA, SectionUSVA, SectionUSCO, SectionUSUT, SectionUSCT,
		SectionUSFL, SectionUSMT, SectionUSOr, SectionUSTX, SectionUSDE,
		SectionUSIA, SectionUSNE, SectionUSNH, SectionUSNJ, SectionUSTN,
		SectionUSMN, SectionUSMD, SectionUSIN, SectionUSKY, SectionUSRI:
		return parseUSStateSection(sectionID, sectionData)
	case SectionTCFEUv2:
		// TCF EU is handled by existing TCF parser
		return nil, nil
	default:
		// Unknown section - skip
		return nil, nil
	}
}

// parseUSNationalSection parses Section 7 (US National)
func parseUSNationalSection(sectionData string) (*USNationalSection, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(sectionData)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(sectionData)
		if err != nil {
			return nil, ErrInvalidGPPEncoding
		}
	}

	if len(decoded) < 8 {
		return nil, ErrInvalidSection
	}

	reader := newBitReader(decoded)

	section := &USNationalSection{
		SensitiveDataProcessing:         make([]OptOutValue, 16),
		KnownChildSensitiveDataConsents: make([]OptOutValue, 3),
	}

	// Version (6 bits)
	section.Version = reader.readInt(6)

	// Sharing Notice (2 bits)
	section.SharingNotice = OptOutValue(reader.readInt(2))

	// Sale Opt-Out Notice (2 bits)
	section.SaleOptOutNotice = OptOutValue(reader.readInt(2))

	// Sharing Opt-Out Notice (2 bits)
	section.SharingOptOutNotice = OptOutValue(reader.readInt(2))

	// Targeted Advertising Opt-Out Notice (2 bits)
	section.TargetedAdvertisingOptOutNotice = OptOutValue(reader.readInt(2))

	// Sensitive Data Processing Opt-Out Notice (2 bits)
	section.SensitiveDataProcessingOptOutNotice = OptOutValue(reader.readInt(2))

	// Sensitive Data Limit Use Notice (2 bits)
	section.SensitiveDataLimitUseNotice = OptOutValue(reader.readInt(2))

	// Sale Opt-Out (2 bits)
	section.SaleOptOut = OptOutValue(reader.readInt(2))

	// Sharing Opt-Out (2 bits)
	section.SharingOptOut = OptOutValue(reader.readInt(2))

	// Targeted Advertising Opt-Out (2 bits)
	section.TargetedAdvertisingOptOut = OptOutValue(reader.readInt(2))

	// Sensitive Data Processing - 16 categories x 2 bits
	sensitiveCategories := 12 // v1 has 12 categories
	if section.Version >= 2 {
		sensitiveCategories = 16 // v2 has 16 categories
	}
	for i := 0; i < sensitiveCategories; i++ {
		section.SensitiveDataProcessing[i] = OptOutValue(reader.readInt(2))
	}

	// Known Child Sensitive Data Consents - 3 age groups (v1) or 2 (v2)
	childCategories := 2
	if section.Version == 1 {
		childCategories = 3
	}
	for i := 0; i < childCategories; i++ {
		section.KnownChildSensitiveDataConsents[i] = OptOutValue(reader.readInt(2))
	}

	// Personal Data Consents (2 bits)
	section.PersonalDataConsents = OptOutValue(reader.readInt(2))

	// MSPA Covered Transaction (2 bits)
	section.MspaCoveredTransaction = OptOutValue(reader.readInt(2))

	// MSPA Opt-Out Option Mode (2 bits)
	section.MspaOptOutOptionMode = OptOutValue(reader.readInt(2))

	// MSPA Service Provider Mode (2 bits)
	section.MspaServiceProviderMode = OptOutValue(reader.readInt(2))

	// GPC (1 bit) - only in v2+
	if section.Version >= 2 {
		section.Gpc = reader.readBool()
	}

	return section, nil
}

// parseUSStateSection parses state-specific sections (8-27)
func parseUSStateSection(sectionID int, sectionData string) (*USStateSection, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(sectionData)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(sectionData)
		if err != nil {
			return nil, ErrInvalidGPPEncoding
		}
	}

	if len(decoded) < 4 {
		return nil, ErrInvalidSection
	}

	reader := newBitReader(decoded)

	section := &USStateSection{
		SectionID:                       sectionID,
		SensitiveDataProcessing:         make([]OptOutValue, 12),
		KnownChildSensitiveDataConsents: make([]OptOutValue, 2),
	}

	// Version (6 bits)
	section.Version = reader.readInt(6)

	// Sale Opt-Out Notice (2 bits)
	section.SaleOptOutNotice = OptOutValue(reader.readInt(2))

	// State-specific fields vary - use California as reference
	// Sharing Opt-Out Notice (2 bits) - CA only
	if sectionID == SectionUSCA {
		section.SharingOptOutNotice = OptOutValue(reader.readInt(2))
	}

	// Targeted Advertising Opt-Out Notice (2 bits)
	section.TargetedAdvertisingOptOutNotice = OptOutValue(reader.readInt(2))

	// Sensitive Data Processing Opt-Out Notice (2 bits)
	section.SensitiveDataProcessingOptOutNotice = OptOutValue(reader.readInt(2))

	// Sale Opt-Out (2 bits)
	section.SaleOptOut = OptOutValue(reader.readInt(2))

	// Sharing Opt-Out (2 bits) - CA only
	if sectionID == SectionUSCA {
		section.SharingOptOut = OptOutValue(reader.readInt(2))
	}

	// Targeted Advertising Opt-Out (2 bits)
	section.TargetedAdvertisingOptOut = OptOutValue(reader.readInt(2))

	// Sensitive Data Processing - varies by state
	sensitiveCategories := getSensitiveCategoriesForState(sectionID)
	for i := 0; i < sensitiveCategories; i++ {
		section.SensitiveDataProcessing[i] = OptOutValue(reader.readInt(2))
	}

	// Known Child Sensitive Data Consents
	childCategories := getChildCategoriesForState(sectionID)
	for i := 0; i < childCategories; i++ {
		section.KnownChildSensitiveDataConsents[i] = OptOutValue(reader.readInt(2))
	}

	// MSPA Covered Transaction (2 bits)
	section.MspaCoveredTransaction = OptOutValue(reader.readInt(2))

	// MSPA Opt-Out Option Mode (2 bits)
	section.MspaOptOutOptionMode = OptOutValue(reader.readInt(2))

	// MSPA Service Provider Mode (2 bits)
	section.MspaServiceProviderMode = OptOutValue(reader.readInt(2))

	// GPC (1 bit)
	section.Gpc = reader.readBool()

	return section, nil
}

// getSensitiveCategoriesForState returns number of sensitive data categories for a state
func getSensitiveCategoriesForState(sectionID int) int {
	switch sectionID {
	case SectionUSCA:
		return 12 // California has 12 categories
	case SectionUSVA, SectionUSCO, SectionUSCT:
		return 8 // These states have 8 categories
	case SectionUSUT:
		return 8 // Utah has 8 categories
	default:
		return 8 // Default for newer states
	}
}

// getChildCategoriesForState returns number of child consent categories for a state
func getChildCategoriesForState(sectionID int) int {
	switch sectionID {
	case SectionUSCA:
		return 2
	default:
		return 2
	}
}

// parseFibonacciIntRange parses Fibonacci-encoded integers from the bit stream
func parseFibonacciIntRange(reader *bitReader) ([]int, error) {
	// The range uses Fibonacci encoding
	// Read until we find the terminator (two consecutive 1s)
	var result []int
	var current int
	var prevBit bool

	// Fibonacci sequence for decoding
	fib := []int{1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233, 377, 610, 987}

	fibIndex := 0
	current = 0

	for i := 0; i < 100; i++ { // Safety limit
		bit := reader.readBool()

		if bit {
			if prevBit {
				// Two consecutive 1s - end of this number
				if current > 0 {
					result = append(result, current)
				}
				current = 0
				fibIndex = 0
				prevBit = false

				// Check if we've reached the end (no more data)
				if reader.bitPos >= len(reader.data)*8-6 {
					break
				}
				continue
			}
			// Add Fibonacci value
			if fibIndex < len(fib) {
				current += fib[fibIndex]
			}
		}

		prevBit = bit
		fibIndex++

		// Safety check
		if fibIndex >= len(fib) {
			if current > 0 {
				result = append(result, current)
			}
			break
		}
	}

	return result, nil
}

// bitReader reads bits from a byte slice
type bitReader struct {
	data   []byte
	bitPos int
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data, bitPos: 0}
}

func (r *bitReader) readBool() bool {
	if r.bitPos/8 >= len(r.data) {
		return false
	}
	bytePos := r.bitPos / 8
	bitOffset := 7 - (r.bitPos % 8)
	r.bitPos++
	return (r.data[bytePos]>>bitOffset)&1 == 1
}

func (r *bitReader) readInt(bits int) int {
	result := 0
	for i := 0; i < bits; i++ {
		result = result << 1
		if r.readBool() {
			result |= 1
		}
	}
	return result
}
