package sgtin

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode"
)

type partitionInfo struct {
	Partition         int64
	CompanyPrefixBits int
	CompanyPrefixDigs int
	ItemRefBits       int
	ItemRefDigs       int
}

var partitionMap = map[int64]partitionInfo{
	0: {0, 40, 12, 4, 1},
	1: {1, 37, 11, 7, 2},
	2: {2, 34, 10, 10, 3},
	3: {3, 30, 9, 14, 4},
	4: {4, 27, 8, 17, 5},
	5: {5, 24, 7, 20, 6},
	6: {6, 20, 6, 24, 7},
}

var companyLenToPartition = map[int]int64{
	12: 0, 11: 1, 10: 2, 9: 3, 8: 4, 7: 5, 6: 6,
}

// SGTINInput : Encode 함수의 입력 구조체
type SGTINInput struct {
	Company string // 업체코드 (6~12자리 숫자)
	ItemRef string // 상품코드
	Serial  string // 일련번호 (숫자 또는 영숫자)
	Filter  int    // Filter Value (0~7)
	Type    string // "96", "198", "auto"
}

// SGTIN : Encode/Decode 함수의 공통 반환 구조체
type SGTIN struct {
	Type      string // "SGTIN-96" 또는 "SGTIN-198"
	Filter    int64
	Partition int64
	Company   string // Company Prefix
	ItemRef   string // Item Reference
	Serial    string // Serial Number
	GTIN      string // Company + ItemRef
	HexEPC    string // EPC Hex 값
}

// Decode : EPC Hex 문자열을 SGTIN 구조체로 디코딩합니다.
func Decode(hexStr string) (*SGTIN, error) {
	hexStr = strings.TrimSpace(hexStr)

	bi := new(big.Int)
	if _, ok := bi.SetString(hexStr, 16); !ok {
		return nil, fmt.Errorf("유효하지 않은 Hex 값: %s", hexStr)
	}

	var bitLen int
	switch len(hexStr) {
	case 24:
		bitLen = 96
	case 52:
		bitLen = 208
	default:
		bitLen = len(hexStr) * 4
	}

	binStr := fmt.Sprintf("%0*b", bitLen, bi)
	if len(binStr) < 8 {
		return nil, fmt.Errorf("데이터가 너무 짧습니다")
	}

	headerInt, _ := strconv.ParseInt(binStr[:8], 2, 64)
	switch headerInt {
	case 48: // 0x30 — SGTIN-96
		result, err := decodeSGTIN96(binStr)
		if err != nil {
			return nil, err
		}
		result.HexEPC = strings.ToUpper(hexStr)
		return result, nil
	case 54: // 0x36 — SGTIN-198
		result, err := decodeSGTIN198(binStr)
		if err != nil {
			return nil, err
		}
		result.HexEPC = strings.ToUpper(hexStr)
		return result, nil
	default:
		return nil, fmt.Errorf("지원하지 않는 헤더입니다 (0x%X)", headerInt)
	}
}

// Encode : SGTINInput 구조체를 SGTIN 구조체(HexEPC 포함)로 인코딩합니다.
func Encode(input SGTINInput) (*SGTIN, error) {
	compLen := len(input.Company)
	partition, exists := companyLenToPartition[compLen]
	if !exists {
		return nil, fmt.Errorf("유효하지 않은 업체코드 길이 (%d자리)", compLen)
	}
	pInfo := partitionMap[partition]

	targetType := "96"
	if input.Type != "auto" && input.Type != "" {
		targetType = input.Type
	} else {
		if !isNumeric(input.Serial) || len(input.Serial) > 11 {
			targetType = "198"
		}
	}

	var binBuilder strings.Builder

	if targetType == "96" {
		binBuilder.WriteString("00110000") // 0x30
	} else {
		binBuilder.WriteString("00110110") // 0x36
	}

	binBuilder.WriteString(fmt.Sprintf("%03b", input.Filter))
	binBuilder.WriteString(fmt.Sprintf("%03b", partition))

	compInt, _ := strconv.ParseInt(input.Company, 10, 64)
	binBuilder.WriteString(fmt.Sprintf("%0*b", pInfo.CompanyPrefixBits, compInt))

	itemInt, _ := strconv.ParseInt(input.ItemRef, 10, 64)
	binBuilder.WriteString(fmt.Sprintf("%0*b", pInfo.ItemRefBits, itemInt))

	if targetType == "96" {
		serInt, ok := new(big.Int).SetString(input.Serial, 10)
		if !ok {
			return nil, fmt.Errorf("SGTIN-96 일련번호는 숫자여야 합니다")
		}
		binBuilder.WriteString(fmt.Sprintf("%038b", serInt))
	} else {
		encodedSerial := encode7BitString(input.Serial)
		binBuilder.WriteString(encodedSerial)
		if padding := 140 - len(encodedSerial); padding > 0 {
			binBuilder.WriteString(strings.Repeat("0", padding))
		}
	}

	fullBin := binBuilder.String()

	var targetBitLen, hexCharLen int
	if targetType == "96" {
		targetBitLen, hexCharLen = 96, 24
	} else {
		targetBitLen, hexCharLen = 208, 52
	}

	if padLen := targetBitLen - len(fullBin); padLen > 0 {
		fullBin += strings.Repeat("0", padLen)
	}

	bi := new(big.Int)
	bi.SetString(fullBin, 2)
	hexStr := fmt.Sprintf("%0*X", hexCharLen, bi)

	return &SGTIN{
		Type:      "SGTIN-" + targetType,
		Filter:    int64(input.Filter),
		Partition: partition,
		Company:   input.Company,
		ItemRef:   input.ItemRef,
		Serial:    input.Serial,
		GTIN:      input.Company + input.ItemRef,
		HexEPC:    hexStr,
	}, nil
}

func decodeSGTIN96(binStr string) (*SGTIN, error) {
	if len(binStr) < 96 {
		return nil, fmt.Errorf("SGTIN-96 data too short: %d bits (96 bits required)", len(binStr))
	}
	if err := validateBinary(binStr[:96]); err != nil {
		return nil, fmt.Errorf("SGTIN-96 invalid binary string: %w", err)
	}

	filter, _ := strconv.ParseInt(binStr[8:11], 2, 64)
	partition, _ := strconv.ParseInt(binStr[11:14], 2, 64)
	pInfo, ok := partitionMap[partition]
	if !ok {
		return nil, fmt.Errorf("invalid partition value: %d", partition)
	}
	cursor := 14

	end := cursor + pInfo.CompanyPrefixBits
	if end > len(binStr) {
		return nil, fmt.Errorf("company prefix bits out of range (cursor=%d, bits=%d, len=%d)", cursor, pInfo.CompanyPrefixBits, len(binStr))
	}
	compVal, _ := strconv.ParseInt(binStr[cursor:end], 2, 64)
	compStr := fmt.Sprintf("%0*d", pInfo.CompanyPrefixDigs, compVal)
	cursor = end

	end = cursor + pInfo.ItemRefBits
	if end > len(binStr) {
		return nil, fmt.Errorf("item reference bits out of range (cursor=%d, bits=%d, len=%d)", cursor, pInfo.ItemRefBits, len(binStr))
	}
	itemVal, _ := strconv.ParseInt(binStr[cursor:end], 2, 64)
	itemStr := fmt.Sprintf("%0*d", pInfo.ItemRefDigs, itemVal)
	cursor = end

	if cursor+38 > len(binStr) {
		return nil, fmt.Errorf("serial bits out of range (cursor=%d, len=%d)", cursor, len(binStr))
	}
	serialVal, _ := strconv.ParseInt(binStr[cursor:cursor+38], 2, 64)

	return &SGTIN{
		Type:      "SGTIN-96",
		Filter:    filter,
		Partition: partition,
		Company:   compStr,
		ItemRef:   itemStr,
		Serial:    fmt.Sprintf("%d", serialVal),
		GTIN:      compStr + itemStr,
	}, nil
}

func decodeSGTIN198(binStr string) (*SGTIN, error) {
	const minBits = 14 + 44 // header(8) + filter(3) + partition(3) + min company+item bits
	if len(binStr) < minBits {
		return nil, fmt.Errorf("SGTIN-198 data too short: %d bits (%d bits required)", len(binStr), minBits)
	}
	if err := validateBinary(binStr); err != nil {
		return nil, fmt.Errorf("SGTIN-198 invalid binary string: %w", err)
	}

	filter, _ := strconv.ParseInt(binStr[8:11], 2, 64)
	partition, _ := strconv.ParseInt(binStr[11:14], 2, 64)
	pInfo, ok := partitionMap[partition]
	if !ok {
		return nil, fmt.Errorf("invalid partition value: %d", partition)
	}
	cursor := 14

	end := cursor + pInfo.CompanyPrefixBits
	if end > len(binStr) {
		return nil, fmt.Errorf("company prefix bits out of range (cursor=%d, bits=%d, len=%d)", cursor, pInfo.CompanyPrefixBits, len(binStr))
	}
	compVal, _ := strconv.ParseInt(binStr[cursor:end], 2, 64)
	compStr := fmt.Sprintf("%0*d", pInfo.CompanyPrefixDigs, compVal)
	cursor = end

	end = cursor + pInfo.ItemRefBits
	if end > len(binStr) {
		return nil, fmt.Errorf("item reference bits out of range (cursor=%d, bits=%d, len=%d)", cursor, pInfo.ItemRefBits, len(binStr))
	}
	itemVal, _ := strconv.ParseInt(binStr[cursor:end], 2, 64)
	itemStr := fmt.Sprintf("%0*d", pInfo.ItemRefDigs, itemVal)
	cursor = end

	serialBits := binStr[cursor:]
	if len(serialBits) > 140 {
		serialBits = serialBits[:140]
	}

	return &SGTIN{
		Type:      "SGTIN-198",
		Filter:    filter,
		Partition: partition,
		Company:   compStr,
		ItemRef:   itemStr,
		Serial:    decode7BitString(serialBits),
		GTIN:      compStr + itemStr,
	}, nil
}

func decode7BitString(bits string) string {
	var sb strings.Builder
	for i := 0; i <= len(bits)-7; i += 7 {
		val, _ := strconv.ParseInt(bits[i:i+7], 2, 64)
		if val == 0 {
			break
		}
		sb.WriteByte(byte(val))
	}
	return sb.String()
}

func encode7BitString(s string) string {
	var sb strings.Builder
	for _, char := range s {
		sb.WriteString(fmt.Sprintf("%07b", char))
	}
	return sb.String()
}

func isNumeric(s string) bool {
	for _, c := range s {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

func validateBinary(s string) error {
	for i, c := range s {
		if c != '0' && c != '1' {
			return fmt.Errorf("invalid character '%c' at position %d", c, i)
		}
	}
	return nil
}
