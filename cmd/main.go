package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	sgtin "github.com/mokhae/epcSGTIN"
)

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
		result, err := sgtin.Decode(strings.TrimSpace(*hexVal))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printResult(result)

	case "encode":
		if *company == "" || *item == "" || *serial == "" {
			fmt.Println("Error: -company, -item, -serial 값이 모두 필요합니다.")
			os.Exit(1)
		}
		result, err := sgtin.Encode(sgtin.SGTINInput{
			Company: *company,
			ItemRef: *item,
			Serial:  *serial,
			Filter:  *filter,
			Type:    *forceType,
		})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("--------------------------------------------------")
		fmt.Printf("Generated EPC (Hex): %s\n", result.HexEPC)
		fmt.Println("--------------------------------------------------")
		printResult(result)

	default:
		fmt.Println("사용법 예시:")
		fmt.Println("  go run ./cmd -mode=encode -company=\"0191862\" -item=\"001012\" -serial=\"NR4338678003\" -filter=0")
		fmt.Println("  go run ./cmd -mode=decode -hex=\"3034657A748014C400000000\"")
		flag.Usage()
	}
}

func printResult(s *sgtin.SGTIN) {
	fmt.Printf("Type:      %s\n", s.Type)
	fmt.Printf("Filter:    %d\n", s.Filter)
	fmt.Printf("Partition: %d\n", s.Partition)
	fmt.Printf("Company:   %s\n", s.Company)
	fmt.Printf("Item Ref:  %s\n", s.ItemRef)
	fmt.Printf("Serial:    %s\n", s.Serial)
	fmt.Printf("GTIN Base: %s\n", s.GTIN)
}
