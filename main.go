package main

import (
	"flag"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// PartitionInfo : GS1 파티션 테이블 구조체
type PartitionInfo struct {
	Partition         int64
	CompanyPrefixBits int
	CompanyPrefixDigs int
	ItemRefBits       int
	ItemRefDigs       int
}

var partitionMap = map[int64]PartitionInfo{
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

func main() {
	mode := flag.String("mode", "", "동작 모드: 'decode' 또는 'encode'")
	hexVal := flag.String("hex", "", "[Decode용] EPC Hex 값")

	company := flag.String("company", "", "[Encode용] 업체코드")
	item := flag.String("item", "", "[Encode용] 상품코드")
	serial := flag.String("serial", "", "[Encode용] 일련번호")
	filter := flag.Int("filter", 0, "[Encode용] Filter Value")
	forceType := flag.String("type", "auto", "[Encode용] '96' 또는 '198' (기본: auto)")

	flag.Parse()

	switch *mode {
	case "decode":
		if *hexVal == "" {
			fmt.Println("Error: -hex 값이 필요합니다.")
			os.Exit(1)
		}
		runDecode(strings.TrimSpace(*hexVal))

	case "encode":
		if *company == "" || *item == "" || *serial == "" {
			fmt.Println("Error: -company, -item, -serial 값이 모두 필요합니다.")
			os.Exit(1)
		}
		runEncode(*company, *item, *serial, *filter, *forceType)

	default:
		fmt.Println("사용법 예시:")
		fmt.Println("  go run main.go -mode=encode -company=\"0191862\" -item=\"001012\" -serial=\"NR4338678003\" -filter=0")
		flag.Usage()
	}
}

// ==========================================
// [로직 1] DECODE (Hex -> Data)
// ==========================================
func runDecode(hexStr string) {
	fmt.Printf(">>> Decoding EPC: %s (Len: %d)\n", hexStr, len(hexStr))

	bi := new(big.Int)
	bi.SetString(hexStr, 16)

	// Hex 길이를 보고 표준 비트 길이 복원
	var bitLen int
	if len(hexStr) == 24 {
		bitLen = 96
	} else if len(hexStr) == 52 {
		bitLen = 208
	} else {
		bitLen = len(hexStr) * 4
	}

	binStr := fmt.Sprintf("%0*b", bitLen, bi)

	if len(binStr) < 8 {
		fmt.Println("Error: 데이터가 너무 짧습니다.")
		return
	}

	headerBin := binStr[:8]
	headerInt, _ := strconv.ParseInt(headerBin, 2, 64)

	if headerInt == 48 { // 0x30
		decodeSGTIN96(binStr)
	} else if headerInt == 54 { // 0x36
		decodeSGTIN198(binStr)
	} else {
		fmt.Printf("Error: 지원하지 않는 헤더입니다 (Hex: %X, Bin: %s)\n", headerInt, headerBin)
	}
}

func decodeSGTIN96(binStr string) {
	filter, _ := strconv.ParseInt(binStr[8:11], 2, 64)
	partition, _ := strconv.ParseInt(binStr[11:14], 2, 64)
	pInfo := partitionMap[partition]
	cursor := 14

	compBits := binStr[cursor : cursor+pInfo.CompanyPrefixBits]
	compVal, _ := strconv.ParseInt(compBits, 2, 64)
	compStr := fmt.Sprintf("%0*d", pInfo.CompanyPrefixDigs, compVal)
	cursor += pInfo.CompanyPrefixBits

	itemBits := binStr[cursor : cursor+pInfo.ItemRefBits]
	itemVal, _ := strconv.ParseInt(itemBits, 2, 64)
	itemStr := fmt.Sprintf("%0*d", pInfo.ItemRefDigs, itemVal)
	cursor += pInfo.ItemRefBits

	serialBits := binStr[cursor : cursor+38]
	serialVal, _ := strconv.ParseInt(serialBits, 2, 64)

	printResult("SGTIN-96", filter, partition, compStr, itemStr, fmt.Sprintf("%d", serialVal))
}

func decodeSGTIN198(binStr string) {
	filter, _ := strconv.ParseInt(binStr[8:11], 2, 64)
	partition, _ := strconv.ParseInt(binStr[11:14], 2, 64)
	pInfo := partitionMap[partition]
	cursor := 14

	compBits := binStr[cursor : cursor+pInfo.CompanyPrefixBits]
	compVal, _ := strconv.ParseInt(compBits, 2, 64)
	compStr := fmt.Sprintf("%0*d", pInfo.CompanyPrefixDigs, compVal)
	cursor += pInfo.CompanyPrefixBits

	itemBits := binStr[cursor : cursor+pInfo.ItemRefBits]
	itemVal, _ := strconv.ParseInt(itemBits, 2, 64)
	itemStr := fmt.Sprintf("%0*d", pInfo.ItemRefDigs, itemVal)
	cursor += pInfo.ItemRefBits

	serialBits := binStr[cursor:]
	if len(serialBits) > 140 {
		serialBits = serialBits[:140]
	}
	serialStr := decode7BitString(serialBits)

	printResult("SGTIN-198", filter, partition, compStr, itemStr, serialStr)
}

// ==========================================
// [로직 2] ENCODE (Data -> Hex)
// ==========================================
func runEncode(company, item, serial string, filter int, forceType string) {
	compLen := len(company)
	partition, exists := companyLenToPartition[compLen]
	if !exists {
		fmt.Printf("Error: 유효하지 않은 업체코드 길이 (%d자리)\n", compLen)
		os.Exit(1)
	}
	pInfo := partitionMap[partition]

	targetType := "96"
	isNumericSerial := isNumeric(serial)
	if forceType != "auto" {
		targetType = forceType
	} else {
		if !isNumericSerial || len(serial) > 11 {
			targetType = "198"
		}
	}

	fmt.Printf(">>> Encoding to %s (Partition: %d)\n", "SGTIN-"+targetType, partition)

	var binBuilder strings.Builder

	// Header
	if targetType == "96" {
		binBuilder.WriteString("00110000") // 0x30
	} else {
		binBuilder.WriteString("00110110") // 0x36
	}

	binBuilder.WriteString(fmt.Sprintf("%03b", filter))
	binBuilder.WriteString(fmt.Sprintf("%03b", partition))

	// Company Prefix
	compInt, _ := strconv.ParseInt(company, 10, 64)
	binBuilder.WriteString(fmt.Sprintf("%0*b", pInfo.CompanyPrefixBits, compInt))

	// Item Reference
	itemInt, _ := strconv.ParseInt(item, 10, 64)
	binBuilder.WriteString(fmt.Sprintf("%0*b", pInfo.ItemRefBits, itemInt))

	// Serial Number
	if targetType == "96" {
		serInt, _ := new(big.Int).SetString(serial, 10)
		binBuilder.WriteString(fmt.Sprintf("%038b", serInt))
	} else {
		encodedSerial := encode7BitString(serial)
		binBuilder.WriteString(encodedSerial)
		padding := 140 - len(encodedSerial)
		if padding > 0 {
			binBuilder.WriteString(strings.Repeat("0", padding))
		}
	}

	fullBin := binBuilder.String()

	// [핵심 수정]
	// Hex 변환 전에 전체 비트 길이를 208비트(SGTIN-198) 또는 96비트(SGTIN-96)로 맞춥니다.
	// 부족한 비트는 "뒤쪽(Right)"에 채워야 메모리 정렬(Left Aligned)이 유지됩니다.
	var targetBitLen int
	var hexCharLen int

	if targetType == "96" {
		targetBitLen = 96
		hexCharLen = 24
	} else {
		targetBitLen = 208 // SGTIN-198은 워드 정렬을 위해 208비트까지 사용
		hexCharLen = 52
	}

	padLen := targetBitLen - len(fullBin)
	if padLen > 0 {
		fullBin += strings.Repeat("0", padLen) // 뒤에 0을 붙임
	}

	bi := new(big.Int)
	bi.SetString(fullBin, 2)

	// 지정된 자릿수로 Hex 출력
	hexStr := fmt.Sprintf("%0*X", hexCharLen, bi)

	fmt.Println("--------------------------------------------------")
	fmt.Printf("Generated EPC (Hex): %s\n", hexStr)
	fmt.Println("--------------------------------------------------")
}

// ---------------- Helper Functions ----------------

func decode7BitString(bits string) string {
	var sb strings.Builder
	for i := 0; i <= len(bits)-7; i += 7 {
		chunk := bits[i : i+7]
		val, _ := strconv.ParseInt(chunk, 2, 64)
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

func printResult(typeStr string, filter, partition int64, comp, item, serial string) {
	fmt.Printf("Type:      %s\n", typeStr)
	fmt.Printf("Filter:    %d\n", filter)
	fmt.Printf("Partition: %d\n", partition)
	fmt.Printf("Company:   %s\n", comp)
	fmt.Printf("Item Ref:  %s\n", item)
	fmt.Printf("Serial:    %s\n", serial)
	fmt.Printf("GTIN Base: %s%s\n", comp, item)
}
